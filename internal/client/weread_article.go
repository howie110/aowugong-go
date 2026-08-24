package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	readability "github.com/mackee/go-readability"
	nethtml "golang.org/x/net/html"
)

const (
	weReadArticleBaseURL    = "https://i.weread.qq.com"
	weReadArticleAppID      = "wxab9b71ad2b90ff34"
	weReadArticleScope      = "snsapi_userinfo,snsapi_timeline,snsapi_friend"
	weReadArticleMaxBody    = 8 << 20
	weReadArticleDevice     = "eink334691225"
	weReadArticleDeviceName = "BOOX"
	weReadArticleUserAgent  = "WeRead/2.1.2 WRBrand/Onyx wr_eink Dalvik/2.1.0 (Linux; U; Android 11; BOOX Build/onyx)"
)

var (
	// ErrWeReadArticleAuth 表示微信读书凭据失效且自动刷新失败。
	ErrWeReadArticleAuth = errors.New("微信读书认证失效")
	// ErrWeReadArticleVerification 表示微信读书要求在官方客户端人工验证。
	ErrWeReadArticleVerification = errors.New("微信读书要求人工验证")
	// ErrWeReadArticleQRExpired 表示当前二维码已经过期。
	ErrWeReadArticleQRExpired = errors.New("微信读书二维码已过期")
	// ErrWeReadArticleQRDeclined 表示用户在微信中拒绝本次登录。
	ErrWeReadArticleQRDeclined = errors.New("微信读书扫码登录已拒绝")
	weReadAccountPattern       = regexp.MustCompile(`^MP_WXS_[0-9]+$`)
	weReadDecimalIdentity      = regexp.MustCompile(`^[0-9]+$`)
)

// WeReadArticleCredentials 描述微信公众号读取所需的完整微信读书凭据。
type WeReadArticleCredentials struct {
	VID          string `json:"vid"`
	DeviceID     string `json:"device_id"`
	InstallID    string `json:"install_id,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Validate 校验微信读书凭据的四个必需字段。
// 输入：接收者是待校验凭据。
// 输出：字段完整返回 nil，否则返回字段错误。
// 副作用：无。
func (c WeReadArticleCredentials) Validate() error {
	// 1. 逐项检查协议后续请求必须使用的字段。
	for name, value := range map[string]string{
		"vid": c.VID, "device_id": c.DeviceID,
		"access_token": c.AccessToken, "refresh_token": c.RefreshToken,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("微信读书凭据字段 %s 为空", name)
		}
	}
	return nil
}

// WeReadArticleQR 描述一次扫码登录二维码。
type WeReadArticleQR struct {
	UUID       string
	ConfirmURL string
}

// WeReadArticleQRPoll 描述二维码的一次轮询结果。
type WeReadArticleQRPoll struct {
	State string
	Code  string
	Last  int
}

// WeReadPublicAccount 描述微信读书书架中的公众号。
type WeReadPublicAccount struct {
	AccountID string
	Title     string
	CoverURL  string
}

// WeReadArticleReference 描述公众号文章列表中的最小引用。
type WeReadArticleReference struct {
	ReviewID   string
	Title      string
	CreateTime int64
}

// WeReadArticleDetail 描述微信读书返回的微信公众号文章详情。
type WeReadArticleDetail struct {
	Title       string
	SourceURL   string
	Author      string
	PublishedAt int64
	Content     string
}

// WeReadArticleClient 负责扫码认证、书架公众号和微信文章读取。
type WeReadArticleClient struct {
	http         *http.Client
	requestMutex sync.Mutex
	lastFinished time.Time
	requestGap   time.Duration
}

// NewWeReadArticleClient 创建固定协议的微信读书文章客户端。
// 输入：httpClient 提供基础 Transport 和超时；为空时使用默认客户端。
// 输出：返回串行限速的客户端。
// 副作用：无，不发起网络请求。
func NewWeReadArticleClient(httpClient *http.Client) *WeReadArticleClient {
	// 1. 复制 HTTP 配置并禁止自动重定向，正文重定向由业务逐次校验。
	configured := &http.Client{Timeout: 30 * time.Second}
	if httpClient != nil {
		*configured = *httpClient
		if configured.Timeout <= 0 {
			configured.Timeout = 30 * time.Second
		}
	}
	configured.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &WeReadArticleClient{http: configured, requestGap: time.Second}
}

// RequestLoginQR 创建微信读书扫码二维码。
// 输入：ctx 控制两次上游请求。
// 输出：返回二维码 UUID 和应编码的微信确认地址。
// 副作用：调用微信读书票据接口和微信开放平台。
func (c *WeReadArticleClient) RequestLoginQR(ctx context.Context) (WeReadArticleQR, error) {
	// 1. 获取微信读书签名票据。
	ticketURL := weReadArticleBaseURL + "/wxticket?nonceStr=weread"
	response, err := c.do(ctx, http.MethodGet, ticketURL, c.versionHeaders(), nil, weReadArticleMaxBody)
	if err != nil {
		return WeReadArticleQR{}, fmt.Errorf("请求微信读书二维码票据: %w", err)
	}
	var ticket struct {
		Signature string `json:"signature"`
		Timestamp int64  `json:"timeStamp"`
	}
	if err := decodeWeReadObject(response.body, &ticket); err != nil {
		return WeReadArticleQR{}, fmt.Errorf("解析微信读书二维码票据: %w", err)
	}
	if ticket.Signature == "" || ticket.Timestamp <= 0 {
		return WeReadArticleQR{}, fmt.Errorf("微信读书二维码票据缺少签名或时间戳")
	}
	if response.status < 200 || response.status >= 300 {
		return WeReadArticleQR{}, fmt.Errorf("微信读书二维码票据 HTTP %d", response.status)
	}

	// 2. 使用固定 AppID 和 scope 换取本次二维码 UUID。
	query := url.Values{
		"appid": {weReadArticleAppID}, "noncestr": {"weread"},
		"timestamp": {strconv.FormatInt(ticket.Timestamp, 10)}, "scope": {weReadArticleScope},
		"signature": {ticket.Signature},
	}
	connectURL := "https://open.weixin.qq.com/connect/sdk/qrconnect?" + query.Encode()
	response, err = c.do(ctx, http.MethodGet, connectURL, map[string]string{"User-Agent": weReadArticleUserAgent}, nil, weReadArticleMaxBody)
	if err != nil {
		return WeReadArticleQR{}, fmt.Errorf("请求微信二维码: %w", err)
	}
	var result struct {
		ErrCode *int   `json:"errcode"`
		UUID    string `json:"uuid"`
	}
	if err := decodeWeReadObject(response.body, &result); err != nil {
		return WeReadArticleQR{}, fmt.Errorf("解析微信二维码: %w", err)
	}
	if response.status < 200 || response.status >= 300 || result.ErrCode == nil || *result.ErrCode != 0 || result.UUID == "" {
		return WeReadArticleQR{}, fmt.Errorf("微信二维码响应异常: HTTP %d, errcode=%v", response.status, result.ErrCode)
	}
	return WeReadArticleQR{UUID: result.UUID, ConfirmURL: "https://open.weixin.qq.com/connect/confirm?uuid=" + url.QueryEscape(result.UUID)}, nil
}

// PollLoginQR 查询一次微信扫码状态。
// 输入：ctx 控制请求，uuid 标识二维码，last 是可选前态。
// 输出：返回 waiting、scanned 或 confirmed；过期和拒绝返回稳定错误。
// 副作用：调用微信二维码长轮询接口。
func (c *WeReadArticleClient) PollLoginQR(ctx context.Context, uuid string, last *int) (WeReadArticleQRPoll, error) {
	// 1. 组合二维码标识和可选前态。
	if strings.TrimSpace(uuid) == "" {
		return WeReadArticleQRPoll{}, fmt.Errorf("微信二维码 UUID 为空")
	}
	query := url.Values{"f": {"json"}, "uuid": {uuid}}
	if last != nil {
		query.Set("last", strconv.Itoa(*last))
	}
	response, err := c.do(ctx, http.MethodGet, "https://long.open.weixin.qq.com/connect/l/qrconnect?"+query.Encode(), map[string]string{"User-Agent": "Mozilla/5.0"}, nil, weReadArticleMaxBody)
	if err != nil {
		return WeReadArticleQRPoll{}, fmt.Errorf("轮询微信二维码: %w", err)
	}
	var result struct {
		ErrCode int    `json:"wx_errcode"`
		Code    string `json:"wx_code"`
	}
	if err := decodeWeReadObject(response.body, &result); err != nil {
		return WeReadArticleQRPoll{}, fmt.Errorf("解析微信二维码状态: %w", err)
	}

	// 2. 只接受微信扫码协议声明的终态和中间态。
	switch result.ErrCode {
	case 408:
		return WeReadArticleQRPoll{State: "waiting", Last: 408}, nil
	case 404:
		return WeReadArticleQRPoll{State: "scanned", Last: 404}, nil
	case 405:
		if result.Code == "" {
			return WeReadArticleQRPoll{}, fmt.Errorf("微信扫码确认缺少交换码")
		}
		return WeReadArticleQRPoll{State: "confirmed", Code: result.Code, Last: 405}, nil
	case 402:
		return WeReadArticleQRPoll{}, ErrWeReadArticleQRExpired
	case 403:
		return WeReadArticleQRPoll{}, ErrWeReadArticleQRDeclined
	default:
		return WeReadArticleQRPoll{}, fmt.Errorf("微信二维码返回未知状态 %d", result.ErrCode)
	}
}

// ExchangeLoginCode 使用微信确认码和既有设备身份换取持久凭据。
// 输入：ctx 控制请求，code 是扫码确认码，previous 是可复用的旧凭据。
// 输出：返回完整微信读书凭据。
// 副作用：首次绑定生成设备身份，重新绑定复用旧身份，并调用微信读书登录接口。
func (c *WeReadArticleClient) ExchangeLoginCode(ctx context.Context, code string, previous *WeReadArticleCredentials) (WeReadArticleCredentials, error) {
	// 1. 首次绑定生成 BOOX 身份，重新绑定沿用同一设备和安装标识。
	deviceID, installID, firstInstall, err := prepareWeReadDevice(previous)
	if err != nil {
		return WeReadArticleCredentials{}, err
	}
	randomValue, err := secureWeReadInt(0, 999)
	if err != nil {
		return WeReadArticleCredentials{}, err
	}
	timestamp := time.Now().UnixMilli()
	signature := sha256.Sum256([]byte(strconv.FormatInt(timestamp, 10) + deviceID + strconv.FormatInt(randomValue, 10)))
	payload := map[string]any{
		"appFirstInstall": firstInstall, "code": code, "deviceId": deviceID, "deviceName": weReadArticleDeviceName,
		"installId": installID, "isAutoLogout": 0, "isFromQrcode": 1, "random": randomValue,
		"signature": hex.EncodeToString(signature[:]), "timestamp": timestamp, "trackId": "", "deviceType": 3,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WeReadArticleCredentials{}, fmt.Errorf("编码微信读书登录请求: %w", err)
	}

	// 2. 交换并验证四项完整凭据。
	headers := c.versionHeaders()
	headers["Content-Type"] = "application/json; charset=UTF-8"
	response, err := c.do(ctx, http.MethodPost, weReadArticleBaseURL+"/login", headers, body, weReadArticleMaxBody)
	if err != nil {
		return WeReadArticleCredentials{}, fmt.Errorf("交换微信读书凭据: %w", err)
	}
	credentials, businessCode, businessMessage, err := decodeWeReadCredentials(response.body, deviceID)
	if err != nil {
		return WeReadArticleCredentials{}, err
	}
	if response.status < 200 || response.status >= 300 || businessCode != 0 {
		return WeReadArticleCredentials{}, fmt.Errorf("微信读书登录被拒绝: HTTP %d, errCode=%d, errMsg=%q: %w", response.status, businessCode, businessMessage, ErrWeReadArticleAuth)
	}
	credentials.InstallID = installID
	return credentials, credentials.Validate()
}

// DiscoverPublicAccounts 读取微信读书书架中的公众号。
// 输入：ctx 控制请求，credentials 会在 Token 过期时原地刷新。
// 输出：返回严格匹配 MP_WXS 的公众号。
// 副作用：调用微信读书书架接口，可能刷新凭据。
func (c *WeReadArticleClient) DiscoverPublicAccounts(ctx context.Context, credentials *WeReadArticleCredentials) ([]WeReadPublicAccount, error) {
	// 1. 读取书架并筛选公众号条目。
	var response struct {
		Books []struct {
			BookID string `json:"bookId"`
			Title  string `json:"title"`
			Cover  string `json:"cover"`
		} `json:"books"`
	}
	if err := c.authenticatedGet(ctx, "/shelf/sync", nil, credentials, &response); err != nil {
		return nil, fmt.Errorf("读取微信读书书架: %w", err)
	}
	if response.Books == nil {
		return nil, fmt.Errorf("微信读书书架响应缺少 books 数组")
	}
	accounts := make([]WeReadPublicAccount, 0)
	for _, book := range response.Books {
		if weReadAccountPattern.MatchString(book.BookID) && strings.TrimSpace(book.Title) != "" {
			accounts = append(accounts, WeReadPublicAccount{AccountID: book.BookID, Title: strings.TrimSpace(book.Title), CoverURL: strings.TrimSpace(book.Cover)})
		}
	}
	return accounts, nil
}

// ListRecentArticles 读取一个公众号最近一页文章引用。
// 输入：ctx 控制请求，credentials 可自动刷新，accountID 是公众号，limit 范围 1 到 100。
// 输出：返回最近文章引用。
// 副作用：调用微信读书公众号文章接口，可能刷新凭据。
func (c *WeReadArticleClient) ListRecentArticles(ctx context.Context, credentials *WeReadArticleCredentials, accountID string, limit int) ([]WeReadArticleReference, error) {
	// 1. 校验公众号和固定单页上限。
	if !weReadAccountPattern.MatchString(accountID) || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("微信读书公众号或文章上限无效")
	}
	var response struct {
		Data []struct {
			ReviewID   string `json:"reviewId"`
			Title      string `json:"title"`
			CreateTime int64  `json:"createTime"`
		} `json:"data"`
		SyncKey *int64 `json:"synckey"`
	}
	query := url.Values{"bookId": {accountID}, "count": {strconv.Itoa(limit)}, "synckey": {"0"}}
	if err := c.authenticatedGet(ctx, "/mp/chapters", query, credentials, &response); err != nil {
		return nil, fmt.Errorf("读取公众号 %s 最近文章: %w", accountID, err)
	}
	if response.Data == nil || response.SyncKey == nil || *response.SyncKey < 0 {
		return nil, fmt.Errorf("公众号 %s 文章页缺少 data 或合法 synckey", accountID)
	}
	articles := make([]WeReadArticleReference, 0, len(response.Data))
	for _, item := range response.Data {
		if strings.TrimSpace(item.ReviewID) == "" {
			return nil, fmt.Errorf("公众号 %s 返回缺少 reviewId 的文章", accountID)
		}
		articles = append(articles, WeReadArticleReference{ReviewID: item.ReviewID, Title: item.Title, CreateTime: item.CreateTime})
	}
	return articles, nil
}

// FetchArticleDetail 读取微信公众号文章的规范元数据。
// 输入：ctx 控制请求，credentials 可自动刷新，reviewID 是文章标识。
// 输出：返回标题、原文地址、作者和发布时间。
// 副作用：调用微信读书文章详情接口，可能刷新凭据。
func (c *WeReadArticleClient) FetchArticleDetail(ctx context.Context, credentials *WeReadArticleCredentials, reviewID string) (WeReadArticleDetail, error) {
	// 1. 请求唯一文章详情接口。
	if strings.TrimSpace(reviewID) == "" {
		return WeReadArticleDetail{}, fmt.Errorf("微信读书文章 reviewId 为空")
	}
	query := url.Values{
		"reviewId": {reviewID}, "commentsCount": {"10"}, "commentsDirection": {"0"},
		"likesCount": {"10"}, "likesDirection": {"0"}, "synckey": {"0"},
	}
	var response struct {
		HTMLContent string `json:"htmlContent"`
		Review      *struct {
			MPInfo *struct {
				DocumentURL string `json:"doc_url"`
				Title       string `json:"title"`
				AccountName string `json:"mp_name"`
				PublishedAt int64  `json:"time"`
				HTMLContent string `json:"htmlContent"`
			} `json:"mpInfo"`
		} `json:"review"`
	}
	if err := c.authenticatedGet(ctx, "/review/single", query, credentials, &response); err != nil {
		return WeReadArticleDetail{}, fmt.Errorf("读取微信读书文章 %s 详情: %w", reviewID, err)
	}
	if response.Review == nil || response.Review.MPInfo == nil {
		return WeReadArticleDetail{}, fmt.Errorf("微信读书文章 %s 缺少公众号详情", reviewID)
	}
	info := response.Review.MPInfo
	if info.DocumentURL == "" || info.Title == "" || info.AccountName == "" || info.PublishedAt <= 0 {
		return WeReadArticleDetail{}, fmt.Errorf("微信读书文章 %s 详情字段不完整", reviewID)
	}
	return WeReadArticleDetail{
		Title: info.Title, SourceURL: info.DocumentURL, Author: info.AccountName, PublishedAt: info.PublishedAt,
		Content: firstNonEmptyWeReadContent(response.HTMLContent, info.HTMLContent),
	}, nil
}

// firstNonEmptyWeReadContent 从微信读书详情候选字段中提取第一份有效正文。
// 输入：values 是可能包含 HTML 或纯文本的正文候选值。
// 输出：返回清理后的第一份非空正文；全部无效时返回空字符串。
// 副作用：无。
func firstNonEmptyWeReadContent(values ...string) string {
	// 1. 按接口稳定性顺序清理候选内容，过滤只有标签或空白的字段。
	for _, value := range values {
		content := htmlToText(value)
		if strings.TrimSpace(content) != "" {
			return content
		}
	}
	return ""
}

// FetchArticleContent 抓取微信公众号原文并提取纯文本正文。
// 输入：ctx 控制请求，sourceURL 必须是 mp.weixin.qq.com 原文地址。
// 输出：返回适合 DeepSeek 分析的纯文本正文和最终地址。
// 副作用：最多访问四次微信公众号页面。
func (c *WeReadArticleClient) FetchArticleContent(ctx context.Context, sourceURL string) (string, string, error) {
	// 1. 逐次验证原文和最多三次重定向。
	current, err := validateWeChatArticleURL(sourceURL)
	if err != nil {
		return "", "", err
	}
	for redirects := 0; redirects <= 3; redirects++ {
		response, err := c.do(ctx, http.MethodGet, current.String(), map[string]string{
			"Accept": "text/html,application/xhtml+xml", "User-Agent": weReadArticleUserAgent,
		}, nil, weReadArticleMaxBody)
		if err != nil {
			return "", "", fmt.Errorf("抓取微信原文: %w", err)
		}
		if response.status >= 300 && response.status < 400 {
			if redirects == 3 || response.header.Get("Location") == "" {
				return "", "", fmt.Errorf("微信原文重定向异常")
			}
			target, parseErr := current.Parse(response.header.Get("Location"))
			if parseErr != nil {
				return "", "", fmt.Errorf("解析微信原文重定向: %w", parseErr)
			}
			current, err = validateWeChatArticleURL(target.String())
			if err != nil {
				return "", "", err
			}
			continue
		}

		// 2. 识别访问挑战并提取唯一 #js_content 文本。
		if response.status < 200 || response.status >= 300 {
			return "", "", fmt.Errorf("微信原文 HTTP %d", response.status)
		}
		mediaType, _, mediaErr := mime.ParseMediaType(response.header.Get("Content-Type"))
		if mediaErr != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
			return "", "", fmt.Errorf("微信原文返回非 HTML 内容")
		}
		bodyText := string(response.body)
		for _, marker := range []string{
			"访问过于频繁", "环境异常", "异常访问", "需要验证", "安全验证",
			"内容已被发布者删除", "该内容已被发布者删除", "此内容因违规无法查看",
		} {
			if strings.Contains(bodyText, marker) {
				return "", "", fmt.Errorf("微信原文触发访问验证: %s", marker)
			}
		}
		document, parseErr := nethtml.Parse(strings.NewReader(bodyText))
		if parseErr != nil {
			return "", "", fmt.Errorf("解析微信原文 HTML: %w", parseErr)
		}
		content, _ := extractWeChatArticleContent(bodyText, document)
		if content == "" {
			return "", "", fmt.Errorf("微信原文缺少可用正文")
		}
		return content, current.String(), nil
	}
	return "", "", fmt.Errorf("微信原文重定向未结束")
}

// extractWeChatArticleContent 按微信专用节点和通用正文算法提取文章正文。
// 输入：bodyText 是原始 HTML，document 是已经解析的 DOM。
// 输出：返回正文文本和命中的解析方式；无法通过质量校验时返回空值。
// 副作用：无，不访问网络。
func extractWeChatArticleContent(bodyText string, document *nethtml.Node) (string, string) {
	// 1. 微信专用节点优先，保留公众号页面最准确的原始顺序。
	if contentNode := findWeChatContentNode(document); contentNode != nil {
		if content := validateWeChatArticleText(collectWeChatText(contentNode), 1); content != "" {
			return content, "wechat_node"
		}
	}

	// 2. 页面结构变化时使用 Mozilla Readability 的 Go 实现提取主内容。
	options := readability.DefaultOptions()
	options.CharThreshold = 120
	options.NbTopCandidates = 8
	article, err := readability.Extract(bodyText, options)
	if err == nil && article.Root != nil {
		if content := validateWeChatArticleText(readability.ExtractTextContent(article.Root), 80); content != "" {
			return content, "readability"
		}
	}
	return "", ""
}

// validateWeChatArticleText 过滤空白、验证页和过短的异常正文。
// 输入：content 是候选正文。
// 输出：通过校验返回规范文本，否则返回空值。
// 副作用：无。
func validateWeChatArticleText(content string, minimumLength int) string {
	content = strings.TrimSpace(strings.Join(strings.Fields(content), " "))
	if len([]rune(content)) < minimumLength {
		return ""
	}
	for _, marker := range []string{"访问过于频繁", "环境异常", "安全验证", "请完成验证", "内容已被发布者删除", "此内容因违规无法查看"} {
		if strings.Contains(content, marker) {
			return ""
		}
	}
	return content
}

type weReadHTTPResponse struct {
	status int
	header http.Header
	body   []byte
}

// authenticatedGet 执行带凭据的微信读书 GET，并在认证拒绝后刷新重放一次。
// 输入：ctx、路径、参数、可变凭据和输出 DTO。
// 输出：成功返回 nil，认证或协议错误返回上下文错误。
// 副作用：调用微信读书一到三次，并可能更新 credentials。
func (c *WeReadArticleClient) authenticatedGet(ctx context.Context, path string, query url.Values, credentials *WeReadArticleCredentials, output any) error {
	// 1. 使用当前凭据请求一次。
	if credentials == nil || credentials.Validate() != nil {
		return fmt.Errorf("微信读书凭据无效: %w", ErrWeReadArticleAuth)
	}
	err := c.authenticatedGetOnce(ctx, path, query, *credentials, output)
	if !errors.Is(err, ErrWeReadArticleAuth) {
		return err
	}

	// 2. 刷新凭据后只重放一次相同请求。
	refreshed, refreshErr := c.RefreshCredentials(ctx, *credentials)
	if refreshErr != nil {
		return refreshErr
	}
	*credentials = refreshed
	if err := c.authenticatedGetOnce(ctx, path, query, refreshed, output); err != nil {
		return fmt.Errorf("刷新后微信读书请求仍失败: %w", err)
	}
	return nil
}

// RefreshCredentials 使用 RefreshToken 更新微信读书凭据。
// 输入：ctx 控制请求，credentials 是旧凭据。
// 输出：返回同账号同设备的新凭据。
// 副作用：调用微信读书登录接口一次。
func (c *WeReadArticleClient) RefreshCredentials(ctx context.Context, credentials WeReadArticleCredentials) (WeReadArticleCredentials, error) {
	// 1. 生成刷新请求签名和固定设备参数。
	if err := credentials.Validate(); err != nil {
		return WeReadArticleCredentials{}, fmt.Errorf("刷新微信读书凭据: %w", err)
	}
	randomValue, err := secureWeReadInt(1, 1000)
	if err != nil {
		return WeReadArticleCredentials{}, err
	}
	timestamp := time.Now().UnixMilli()
	signature := sha256.Sum256([]byte(strconv.FormatInt(timestamp, 10) + credentials.DeviceID + strconv.FormatInt(randomValue, 10)))
	payload := map[string]any{
		"deviceId": credentials.DeviceID, "deviceName": weReadArticleDeviceName, "inBackground": 0, "kickType": 1,
		"random": randomValue, "refCgi": "", "refreshToken": credentials.RefreshToken,
		"signature": hex.EncodeToString(signature[:]), "timestamp": timestamp, "trackId": "", "deviceType": 3,
	}
	if credentials.InstallID != "" {
		payload["installId"] = credentials.InstallID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WeReadArticleCredentials{}, fmt.Errorf("编码微信读书刷新请求: %w", err)
	}
	headers := c.versionHeaders()
	headers["Content-Type"] = "application/json; charset=UTF-8"
	response, err := c.do(ctx, http.MethodPost, weReadArticleBaseURL+"/login", headers, body, weReadArticleMaxBody)
	if err != nil {
		return WeReadArticleCredentials{}, fmt.Errorf("刷新微信读书凭据: %w", err)
	}
	refreshed, businessCode, businessMessage, err := decodeWeReadCredentials(response.body, credentials.DeviceID)
	if err != nil {
		return WeReadArticleCredentials{}, err
	}
	if businessCode == -2041 {
		return WeReadArticleCredentials{}, ErrWeReadArticleVerification
	}
	if response.status < 200 || response.status >= 300 || businessCode != 0 {
		return WeReadArticleCredentials{}, fmt.Errorf("微信读书刷新被拒绝: errCode=%d, errMsg=%q: %w", businessCode, businessMessage, ErrWeReadArticleAuth)
	}
	if refreshed.VID != credentials.VID || refreshed.DeviceID != credentials.DeviceID {
		return WeReadArticleCredentials{}, fmt.Errorf("微信读书刷新返回了不同账号或设备")
	}
	// 2. 微信读书可能只轮换 AccessToken；未返回新 RefreshToken 时继续使用旧值。
	if strings.TrimSpace(refreshed.RefreshToken) == "" {
		refreshed.RefreshToken = credentials.RefreshToken
	}
	refreshed.InstallID = credentials.InstallID
	return refreshed, refreshed.Validate()
}

// authenticatedGetOnce 执行恰好一次微信读书认证 GET。
// 输入：ctx、路径、参数、凭据和输出 DTO。
// 输出：认证拒绝返回 ErrWeReadArticleAuth，成功解码 output。
// 副作用：访问微信读书一次。
func (c *WeReadArticleClient) authenticatedGetOnce(ctx context.Context, path string, query url.Values, credentials WeReadArticleCredentials, output any) error {
	// 1. 构造固定 i.weread.qq.com 地址和认证请求头。
	parsed, err := url.Parse(weReadArticleBaseURL + path)
	if err != nil || parsed.Host != "i.weread.qq.com" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("微信读书请求路径无效")
	}
	if query != nil {
		parsed.RawQuery = query.Encode()
	}
	headers := c.versionHeaders()
	headers["vid"] = credentials.VID
	headers["accessToken"] = credentials.AccessToken
	response, err := c.do(ctx, http.MethodGet, parsed.String(), headers, nil, weReadArticleMaxBody)
	if err != nil {
		return err
	}
	if response.status == http.StatusUnauthorized {
		return ErrWeReadArticleAuth
	}
	var envelope struct {
		ErrCode *int   `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
	}
	if err := decodeWeReadObject(response.body, &envelope); err != nil {
		return fmt.Errorf("解析微信读书 %s 响应: %w", path, err)
	}
	if envelope.ErrCode != nil && *envelope.ErrCode != 0 {
		if *envelope.ErrCode == -2012 {
			return ErrWeReadArticleAuth
		}
		if *envelope.ErrCode == -2041 {
			return ErrWeReadArticleVerification
		}
		return fmt.Errorf("微信读书 %s 返回 errCode=%d, errMsg=%q", path, *envelope.ErrCode, envelope.ErrMsg)
	}
	if response.status < 200 || response.status >= 300 {
		return fmt.Errorf("微信读书 %s HTTP %d", path, response.status)
	}
	if err := json.Unmarshal(response.body, output); err != nil {
		return fmt.Errorf("解码微信读书 %s 数据: %w", path, err)
	}
	return nil
}

// do 串行限速执行一次上游 HTTP 请求。
// 输入：ctx、方法、地址、请求头、正文和最大响应大小。
// 输出：返回完整响应或网络错误。
// 副作用：等待限速并访问外部网络。
func (c *WeReadArticleClient) do(ctx context.Context, method, address string, headers map[string]string, body []byte, maximumBytes int64) (weReadHTTPResponse, error) {
	// 1. 串行等待距离上一次请求结束至少一秒。
	c.requestMutex.Lock()
	defer c.requestMutex.Unlock()
	if remaining := c.requestGap - time.Since(c.lastFinished); !c.lastFinished.IsZero() && remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return weReadHTTPResponse{}, fmt.Errorf("等待微信读书请求限速: %w", ctx.Err())
		case <-timer.C:
		}
	}

	// 2. 发送请求并按大小上限完整读取响应。
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return weReadHTTPResponse{}, fmt.Errorf("创建微信读书请求: %w", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := c.http.Do(request)
	c.lastFinished = time.Now()
	if err != nil {
		return weReadHTTPResponse{}, fmt.Errorf("执行微信读书请求: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumBytes+1))
	if err != nil {
		return weReadHTTPResponse{}, fmt.Errorf("读取微信读书响应: %w", err)
	}
	if int64(len(content)) > maximumBytes {
		return weReadHTTPResponse{}, fmt.Errorf("微信读书响应超过 %d 字节", maximumBytes)
	}
	return weReadHTTPResponse{status: response.StatusCode, header: response.Header.Clone(), body: content}, nil
}

// versionHeaders 返回固定微信读书客户端身份请求头。
// 输入：无。
// 输出：返回一份可修改的新 map。
// 副作用：无。
func (c *WeReadArticleClient) versionHeaders() map[string]string {
	// 1. 使用唯一经过验证的墨水屏客户端身份。
	return map[string]string{
		"User-Agent": weReadArticleUserAgent, "baseapi": "30", "appver": "2.1.2.10245900",
		"basever": "2.1.2.10245900", "osver": "11", "channelId": "900",
	}
}

type weReadIdentity string

// UnmarshalJSON 兼容微信读书以字符串或整数返回账号标识。
// 输入：data 是 JSON 标量。
// 输出：把规范账号写入接收者。
// 副作用：修改接收者。
func (i *weReadIdentity) UnmarshalJSON(data []byte) error {
	// 1. 字符串直接解码，数值只接受十进制整数。
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil || value == "" {
			return fmt.Errorf("微信读书账号标识字符串无效")
		}
		*i = weReadIdentity(value)
		return nil
	}
	value := string(bytes.TrimSpace(data))
	if !weReadDecimalIdentity.MatchString(value) {
		return fmt.Errorf("微信读书账号标识无效")
	}
	*i = weReadIdentity(value)
	return nil
}

// decodeWeReadCredentials 解码登录或刷新响应中的凭据。
// 输入：body 是 JSON，deviceID 是本地稳定设备号。
// 输出：返回凭据、业务代码、业务消息和协议错误。
// 副作用：无。
func decodeWeReadCredentials(body []byte, deviceID string) (WeReadArticleCredentials, int, string, error) {
	// 1. 解码兼容字符串和整数形式的 vid。
	var response struct {
		VID          weReadIdentity `json:"vid"`
		AccessToken  string         `json:"accessToken"`
		RefreshToken string         `json:"refreshToken"`
		ErrCode      *int           `json:"errCode"`
		ErrMsg       string         `json:"errMsg"`
	}
	if err := decodeWeReadObject(body, &response); err != nil {
		return WeReadArticleCredentials{}, 0, "", fmt.Errorf("解析微信读书凭据响应: %w", err)
	}
	code := 0
	if response.ErrCode != nil {
		code = *response.ErrCode
	}
	return WeReadArticleCredentials{
		VID: string(response.VID), DeviceID: deviceID,
		AccessToken: response.AccessToken, RefreshToken: response.RefreshToken,
	}, code, response.ErrMsg, nil
}

// decodeWeReadObject 要求响应是 JSON 对象并解码到目标结构。
// 输入：body 是响应，output 是结构体指针。
// 输出：合法对象返回 nil。
// 副作用：修改 output。
func decodeWeReadObject(body []byte, output any) error {
	// 1. 拒绝空响应、数组和标量。
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("响应不是 JSON 对象")
	}
	return json.Unmarshal(trimmed, output)
}

// newWeReadDeviceID 生成绑定后持久使用的设备 ID。
// 输入：无。
// 输出：返回固定前缀和十九位随机数。
// 副作用：读取系统随机源。
func newWeReadDeviceID() (string, error) {
	// 1. 读取无符号 63 位随机数并固定宽度。
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("生成微信读书设备 ID: %w", err)
	}
	value := binary.BigEndian.Uint64(buffer) & uint64(^uint64(0)>>1)
	return fmt.Sprintf("%s%019d", weReadArticleDevice, value), nil
}

// newWeReadInstallID 生成一次登录使用的安装 ID。
// 输入：无。
// 输出：返回固定前缀和二十六位随机数字。
// 副作用：读取系统随机源。
func newWeReadInstallID() (string, error) {
	// 1. 逐位生成，避免弱伪随机源。
	digits := make([]byte, 26)
	for index := range digits {
		value, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("生成微信读书安装 ID: %w", err)
		}
		digits[index] = byte('0' + value.Int64())
	}
	return "eink31" + string(digits), nil
}

// prepareWeReadDevice 准备首次登录或重新绑定使用的稳定 BOOX 设备身份。
// 输入：previous 是可选旧凭据，包含已经向微信读书登记的设备标识。
// 输出：返回设备 ID、安装 ID、首次安装标记和错误。
// 副作用：缺少字段时读取系统随机源生成一次并交由调用方持久保存。
func prepareWeReadDevice(previous *WeReadArticleCredentials) (string, string, int, error) {
	// 1. 重新绑定必须复用旧设备 ID，旧版凭据没有安装 ID 时只补生成一次。
	if previous != nil && strings.TrimSpace(previous.DeviceID) != "" {
		installID := strings.TrimSpace(previous.InstallID)
		if installID == "" {
			var err error
			installID, err = newWeReadInstallID()
			if err != nil {
				return "", "", 0, err
			}
		}
		return strings.TrimSpace(previous.DeviceID), installID, 0, nil
	}

	// 2. 首次绑定同时生成设备和安装标识，后续随加密凭据保存。
	deviceID, err := newWeReadDeviceID()
	if err != nil {
		return "", "", 0, err
	}
	installID, err := newWeReadInstallID()
	if err != nil {
		return "", "", 0, err
	}
	return deviceID, installID, 1, nil
}

// secureWeReadInt 生成闭区间密码学随机整数。
// 输入：minimum 和 maximum 是闭区间边界。
// 输出：返回区间内整数。
// 副作用：读取系统随机源。
func secureWeReadInt(minimum, maximum int64) (int64, error) {
	// 1. 校验区间并在区间宽度内取随机数。
	if minimum > maximum {
		return 0, fmt.Errorf("微信读书随机数区间无效")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
	if err != nil {
		return 0, fmt.Errorf("生成微信读书随机数: %w", err)
	}
	return minimum + value.Int64(), nil
}

// validateWeChatArticleURL 校验微信公众号原文地址。
// 输入：value 是原文或重定向地址。
// 输出：只接受无凭据、无端口的 mp.weixin.qq.com HTTPS 地址。
// 副作用：无。
func validateWeChatArticleURL(value string) (*url.URL, error) {
	// 1. 严格限制主机、协议和路径，避免正文抓取成为通用代理。
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("解析微信原文地址: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "mp.weixin.qq.com" || parsed.Port() != "" || parsed.User != nil || (parsed.Path != "/s" && !strings.HasPrefix(parsed.Path, "/s/")) {
		return nil, fmt.Errorf("微信原文地址不在允许范围")
	}
	parsed.Fragment = ""
	return parsed, nil
}

// findWeChatContentNode 查找微信公众号正文节点。
// 输入：node 是 HTML 树根。
// 输出：返回 id=js_content 的节点或 nil。
// 副作用：无。
func findWeChatContentNode(node *nethtml.Node) *nethtml.Node {
	// 1. 优先使用经典正文 ID，再兼容新版正文类名和旧版文章容器。
	if found := findWeChatNode(node, "id", "js_content", false); found != nil {
		return found
	}
	if found := findWeChatNode(node, "class", "rich_media_content", true); found != nil {
		return found
	}
	return findWeChatNode(node, "id", "img-content", false)
}

// findWeChatNode 按属性深度优先查找微信公众号正文候选节点。
// 输入：node 是根节点，key 和 value 是属性条件，tokenMatch 控制是否按类名词元匹配。
// 输出：返回首个匹配节点；不存在时返回 nil。
// 副作用：无。
func findWeChatNode(node *nethtml.Node, key, value string, tokenMatch bool) *nethtml.Node {
	// 1. 检查当前元素属性，类名使用空白词元避免误命中相似名称。
	if node.Type == nethtml.ElementNode {
		for _, attribute := range node.Attr {
			matched := attribute.Key == key && attribute.Val == value
			if tokenMatch && attribute.Key == key {
				matched = slices.Contains(strings.Fields(attribute.Val), value)
			}
			if matched {
				return node
			}
		}
	}

	// 2. 按 DOM 顺序递归查找子节点。
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findWeChatNode(child, key, value, tokenMatch); found != nil {
			return found
		}
	}
	return nil
}

// collectWeChatText 提取正文节点中的可见纯文本。
// 输入：node 是正文 HTML 节点。
// 输出：返回压缩空白后的文本。
// 副作用：无。
func collectWeChatText(node *nethtml.Node) string {
	// 1. 忽略脚本和样式，按 DOM 顺序收集文本节点。
	parts := make([]string, 0)
	var visit func(*nethtml.Node)
	visit = func(current *nethtml.Node) {
		if current.Type == nethtml.ElementNode && (current.Data == "script" || current.Data == "style") {
			return
		}
		if current.Type == nethtml.TextNode {
			if value := strings.TrimSpace(current.Data); value != "" {
				parts = append(parts, value)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}
