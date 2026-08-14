package articleanalysis

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	weReadLoginLifetime     = 5 * time.Minute
	weReadRecentPerAccount  = 20
	weReadCredentialVersion = 1
	weReadCredentialAAD     = "aowugong/weread-article/1"
)

type weReadLoginFlow struct {
	state      string
	message    string
	uuid       string
	confirmURL string
	last       *int
	expiresAt  time.Time
}

type weReadListedArticle struct {
	accountID string
	reference client.WeReadArticleReference
}

// WeReadSource 把微信读书书架公众号转换为现有投资文章来源。
type WeReadSource struct {
	repository     *Repository
	client         *client.WeReadArticleClient
	encryptionKey  [32]byte
	operationMutex sync.Mutex
	flowMutex      sync.Mutex
	flow           *weReadLoginFlow
}

// NewWeReadSource 创建微信读书投资文章来源。
// 输入：repository 访问现有 PostgreSQL，weReadClient 访问上游，encryptionSecret 来自应用加密配置。
// 输出：返回可同时用于绑定和文章抓取的来源。
// 副作用：无。
func NewWeReadSource(repository *Repository, weReadClient *client.WeReadArticleClient, encryptionSecret string) *WeReadSource {
	// 1. 使用 SHA-256 把现有应用密钥稳定派生为 AES-256 密钥。
	return &WeReadSource{
		repository: repository, client: weReadClient,
		encryptionKey: sha256.Sum256([]byte(encryptionSecret)),
	}
}

// Binding 返回文章抓取页的微信读书连接和公众号状态。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回连接状态和全部已发现公众号。
// 副作用：只读 PostgreSQL 和内存扫码状态。
func (s *WeReadSource) Binding(ctx context.Context) (WeReadBinding, error) {
	// 1. 读取持久凭据和公众号，不访问微信读书上游。
	_, _, found, err := s.repository.loadWeReadCredentialCiphertext(ctx)
	if err != nil {
		return WeReadBinding{}, err
	}
	accounts, err := s.repository.listWeReadAccounts(ctx, false)
	if err != nil {
		return WeReadBinding{}, err
	}
	state, message := "disconnected", "尚未绑定微信读书"
	if found {
		state, message = "connected", "微信读书凭据已保存"
		if _, credentialErr := s.loadCredentials(ctx); credentialErr != nil {
			state, message = "failed", "微信读书凭据不可用，请重新绑定"
		} else {
			fetchStatus, fetchMessage, statusErr := s.repository.weReadSourceFetchStatus(ctx)
			if statusErr != nil {
				return WeReadBinding{}, statusErr
			}
			if fetchStatus == "error" {
				state, message = "degraded", "最近抓取存在异常，请查看任务记录"
				if strings.Contains(fetchMessage, "refresh_token") || strings.Contains(fetchMessage, "凭据") {
					state = "failed"
					message = "微信读书凭据不可用，请重新绑定"
				}
			}
		}
	}

	// 2. 未连接时优先展示仍有效的扫码中间态。
	if !found {
		s.flowMutex.Lock()
		if s.flow != nil {
			state, message = s.flow.state, s.flow.message
		}
		s.flowMutex.Unlock()
	}
	return WeReadBinding{State: state, Message: message, Accounts: accounts}, nil
}

// StartLogin 创建或复用一个五分钟微信读书扫码流程。
// 输入：ctx 控制二维码请求。
// 输出：返回公开扫码状态。
// 副作用：调用微信上游并修改内存流程。
func (s *WeReadSource) StartLogin(ctx context.Context) (WeReadLoginStatus, error) {
	// 1. 有效流程直接复用，避免页面重复点击生成多个二维码。
	s.flowMutex.Lock()
	if s.flow != nil && time.Now().Before(s.flow.expiresAt) && (s.flow.state == "waiting" || s.flow.state == "scanned") {
		status := publicWeReadLoginStatus(s.flow)
		s.flowMutex.Unlock()
		return status, nil
	}
	s.flowMutex.Unlock()

	// 2. 串行请求二维码并替换终止流程。
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	s.flowMutex.Lock()
	if s.flow != nil && time.Now().Before(s.flow.expiresAt) && (s.flow.state == "waiting" || s.flow.state == "scanned") {
		status := publicWeReadLoginStatus(s.flow)
		s.flowMutex.Unlock()
		return status, nil
	}
	s.flowMutex.Unlock()
	request, err := s.client.RequestLoginQR(ctx)
	if err != nil {
		return WeReadLoginStatus{}, fmt.Errorf("创建微信读书扫码登录: %w", err)
	}
	flow := &weReadLoginFlow{
		state: "waiting", message: "等待微信扫描", uuid: request.UUID,
		confirmURL: request.ConfirmURL, expiresAt: time.Now().Add(weReadLoginLifetime),
	}
	s.flowMutex.Lock()
	s.flow = flow
	s.flowMutex.Unlock()
	return publicWeReadLoginStatus(flow), nil
}

// PollLogin 推进当前扫码流程一次。
// 输入：ctx 控制轮询、凭据交换和书架发现。
// 输出：返回最新公开状态。
// 副作用：调用微信上游，成功时加密写 PostgreSQL 并发现公众号。
func (s *WeReadSource) PollLogin(ctx context.Context) (WeReadLoginStatus, error) {
	// 1. 读取当前流程并在本地处理超时。
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	s.flowMutex.Lock()
	flow := s.flow
	if flow == nil {
		s.flowMutex.Unlock()
		return WeReadLoginStatus{}, fmt.Errorf("当前没有微信读书扫码流程")
	}
	if flow.state != "waiting" && flow.state != "scanned" {
		status := publicWeReadLoginStatus(flow)
		s.flowMutex.Unlock()
		return status, nil
	}
	if time.Now().After(flow.expiresAt) {
		flow.state, flow.message, flow.uuid, flow.confirmURL = "expired", "二维码已过期", "", ""
		status := publicWeReadLoginStatus(flow)
		s.flowMutex.Unlock()
		return status, nil
	}
	uuid, last := flow.uuid, flow.last
	s.flowMutex.Unlock()

	// 2. 轮询微信并把中间态写回同一个流程。
	result, err := s.client.PollLoginQR(ctx, uuid, last)
	if err != nil {
		state, message := "failed", err.Error()
		if errors.Is(err, client.ErrWeReadArticleQRExpired) {
			state, message = "expired", "二维码已过期"
		}
		if errors.Is(err, client.ErrWeReadArticleQRDeclined) {
			state, message = "declined", "已在微信中拒绝登录"
		}
		s.updateLoginFlow(flow, state, message, true)
		return publicWeReadLoginStatus(flow), nil
	}
	lastValue := result.Last
	s.flowMutex.Lock()
	if s.flow == flow {
		flow.last = &lastValue
	}
	s.flowMutex.Unlock()
	if result.State != "confirmed" {
		message := "等待微信扫描"
		if result.State == "scanned" {
			message = "已扫描，请在手机中确认"
		}
		s.updateLoginFlow(flow, result.State, message, false)
		return publicWeReadLoginStatus(flow), nil
	}

	// 3. 确认后交换、加密保存凭据，并立即发现书架公众号。
	credentials, err := s.client.ExchangeLoginCode(ctx, result.Code)
	if err != nil {
		s.updateLoginFlow(flow, "failed", "微信读书登录失败", true)
		return publicWeReadLoginStatus(flow), fmt.Errorf("交换微信读书登录凭据: %w", err)
	}
	if err := s.saveCredentials(ctx, credentials); err != nil {
		s.updateLoginFlow(flow, "failed", "保存微信读书凭据失败", true)
		return publicWeReadLoginStatus(flow), err
	}
	if err := s.repository.SetWeReadSourceActive(ctx, true); err != nil {
		s.updateLoginFlow(flow, "failed", "启用微信读书文章来源失败", true)
		return publicWeReadLoginStatus(flow), err
	}
	if _, err := s.refreshAccountsWithCredentials(ctx, credentials); err != nil {
		s.updateLoginFlow(flow, "connected", "凭据已保存，读取书架公众号失败", true)
		return publicWeReadLoginStatus(flow), nil
	}
	if err := s.repository.clearWeReadSourceFetchError(ctx); err != nil {
		s.updateLoginFlow(flow, "failed", "更新微信读书来源状态失败", true)
		return publicWeReadLoginStatus(flow), err
	}
	s.updateLoginFlow(flow, "connected", "微信读书凭据已保存", true)
	return publicWeReadLoginStatus(flow), nil
}

// LoginQRPNG 返回当前扫码流程的二维码 PNG。
// 输入：无。
// 输出：返回 320 像素二维码；流程无效时返回错误。
// 副作用：读取内存流程并编码图片。
func (s *WeReadSource) LoginQRPNG() ([]byte, error) {
	// 1. 复制仍有效的确认地址，避免编码期间持锁。
	s.flowMutex.Lock()
	if s.flow == nil || s.flow.confirmURL == "" || time.Now().After(s.flow.expiresAt) {
		s.flowMutex.Unlock()
		return nil, fmt.Errorf("微信读书二维码不存在或已过期")
	}
	confirmation := s.flow.confirmURL
	s.flowMutex.Unlock()
	content, err := qrcode.Encode(confirmation, qrcode.Medium, 320)
	if err != nil {
		return nil, fmt.Errorf("生成微信读书二维码: %w", err)
	}
	return content, nil
}

// RefreshAccounts 重新读取当前微信读书书架公众号。
// 输入：ctx 控制凭据和上游访问。
// 输出：返回最新公众号列表。
// 副作用：调用微信读书，可能刷新凭据并写 PostgreSQL。
func (s *WeReadSource) RefreshAccounts(ctx context.Context) ([]WeReadAccount, error) {
	// 1. 串行加载凭据并执行书架发现。
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	credentials, err := s.loadCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return s.refreshAccountsWithCredentials(ctx, credentials)
}

// SetAccountEnabled 更新一个公众号是否参与投资文章抓取。
// 输入：ctx 控制写入，accountID 是公众号，enabled 是目标状态。
// 输出：返回更新错误。
// 副作用：写 PostgreSQL。
func (s *WeReadSource) SetAccountEnabled(ctx context.Context, accountID string, enabled bool) error {
	// 1. 只允许书架发现过的 MP_WXS 公众号。
	if !strings.HasPrefix(accountID, "MP_WXS_") {
		return fmt.Errorf("微信读书公众号 ID 无效")
	}
	return s.repository.setWeReadAccountEnabled(ctx, accountID, enabled)
}

// Fetch 从所有已启用书架公众号读取最近新文章。
// 输入：ctx 控制处理，sourceID 是现有聚合来源，feedURL 保留接口兼容，limit 限制单公众号检查数。
// 输出：返回可直接写入 investment_article 的文章；部分文章失败时同时返回成功项和汇总错误。
// 副作用：读取并可能更新 PostgreSQL，调用微信读书和微信公众号原文。
func (s *WeReadSource) Fetch(ctx context.Context, sourceID int64, _ string, limit int) ([]client.ArticleItem, error) {
	// 1. 串行加载凭据和已启用公众号，控制单公众号最多检查二十篇。
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	accounts, err := s.repository.listWeReadAccounts(ctx, true)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("尚未启用任何微信读书公众号")
	}
	now := time.Now()
	dueAccounts := dueWeReadAccounts(accounts, now)
	if len(dueAccounts) == 0 {
		return nil, nil
	}
	credentials, err := s.loadCredentials(ctx)
	if err != nil {
		return nil, err
	}
	originalCredentials := credentials
	// 2. 每个公众号只读取最近一页并按 reviewId 去重。
	listed := make([]weReadListedArticle, 0, len(dueAccounts)*weReadRecentPerAccount)
	failures := make([]string, 0)
	seen := make(map[string]struct{})
	for _, account := range dueAccounts {
		perAccount := account.FetchLimit
		if perAccount < 1 {
			perAccount = weReadRecentPerAccount
		}
		if limit > 0 && limit < perAccount {
			perAccount = limit
		}
		references, listErr := s.client.ListRecentArticles(ctx, &credentials, account.AccountID, perAccount)
		if listErr != nil {
			failures = append(failures, account.Title+": "+listErr.Error())
			continue
		}
		if markErr := s.repository.markWeReadAccountChecked(ctx, account.AccountID, now); markErr != nil {
			failures = append(failures, account.Title+": "+markErr.Error())
		}
		for _, reference := range references {
			if _, exists := seen[reference.ReviewID]; exists {
				continue
			}
			seen[reference.ReviewID] = struct{}{}
			listed = append(listed, weReadListedArticle{accountID: account.AccountID, reference: reference})
		}
	}
	externalIDs := make([]string, 0, len(listed))
	for _, item := range listed {
		externalIDs = append(externalIDs, item.reference.ReviewID)
	}
	existing, err := s.repository.existingWeReadExternalIDs(ctx, sourceID, externalIDs)
	if err != nil {
		return nil, err
	}

	// 3. 只对数据库未知文章读取详情和原文；历史同链接文章直接绑定 reviewId。
	items := make([]client.ArticleItem, 0)
	for _, listedArticle := range listed {
		reference := listedArticle.reference
		if _, exists := existing[reference.ReviewID]; exists {
			continue
		}
		detail, detailErr := s.client.FetchArticleDetail(ctx, &credentials, reference.ReviewID)
		if detailErr != nil {
			failures = append(failures, reference.Title+": "+detailErr.Error())
			continue
		}
		bound, bindErr := s.repository.bindWeReadExternalIDByLink(ctx, sourceID, detail.SourceURL, reference.ReviewID)
		if bindErr != nil {
			return items, bindErr
		}
		if bound {
			continue
		}
		content, finalURL, contentErr := s.client.FetchArticleContent(ctx, detail.SourceURL)
		if contentErr != nil {
			content = weReadDetailContentFallback(detail.Title)
			finalURL = detail.SourceURL
			if content == "" {
				failures = append(failures, weReadArticleTitle(reference.Title, detail.Title)+": "+contentErr.Error())
				continue
			}
		}
		items = append(items, client.ArticleItem{
			ArticleKey: weReadArticleKey(sourceID, reference.ReviewID), ExternalID: reference.ReviewID,
			Title: weReadArticleTitle(reference.Title, detail.Title), Link: finalURL, Author: strings.TrimSpace(detail.Author),
			PublishedAt: weReadPublishedAt(detail.PublishedAt), Summary: truncateRunes(content, 300), Content: content,
			RawEntry: map[string]any{"review_id": reference.ReviewID, "account_id": listedArticle.accountID},
		})
	}

	// 4. 自动刷新发生时先持久化新凭据，再返回成功项和可通知的部分失败。
	if credentials != originalCredentials {
		if saveErr := s.saveCredentials(ctx, credentials); saveErr != nil {
			failures = append(failures, saveErr.Error())
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return items, fmt.Errorf("微信读书文章读取部分失败: %s", strings.Join(failures, "；"))
	}
	return items, nil
}

// refreshAccountsWithCredentials 使用给定凭据发现书架公众号。
// 输入：ctx 控制访问，credentials 是当前凭据。
// 输出：返回数据库中的最新完整公众号列表。
// 副作用：调用微信读书，可能更新凭据和公众号表。
func (s *WeReadSource) refreshAccountsWithCredentials(ctx context.Context, credentials client.WeReadArticleCredentials) ([]WeReadAccount, error) {
	// 1. 调用书架接口并在 Token 刷新后保存新凭据。
	original := credentials
	discovered, err := s.client.DiscoverPublicAccounts(ctx, &credentials)
	if credentials != original {
		if saveErr := s.saveCredentials(ctx, credentials); saveErr != nil {
			return nil, saveErr
		}
	}
	if err != nil {
		return nil, err
	}
	accounts := make([]WeReadAccount, 0, len(discovered))
	for _, account := range discovered {
		accounts = append(accounts, WeReadAccount{AccountID: account.AccountID, Title: account.Title, CoverURL: account.CoverURL})
	}
	if err := s.repository.upsertWeReadAccounts(ctx, accounts); err != nil {
		return nil, err
	}
	return s.repository.listWeReadAccounts(ctx, false)
}

// saveCredentials 加密并保存微信读书凭据。
// 输入：ctx 控制写入，credentials 是待保存明文。
// 输出：返回加密或写入错误。
// 副作用：读取系统随机源并写 PostgreSQL。
func (s *WeReadSource) saveCredentials(ctx context.Context, credentials client.WeReadArticleCredentials) error {
	// 1. 序列化后使用 AES-256-GCM 和固定 AAD 认证加密。
	if err := credentials.Validate(); err != nil {
		return err
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("编码微信读书凭据: %w", err)
	}
	gcm, err := s.credentialGCM()
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("生成微信读书凭据 nonce: %w", err)
	}
	envelope := append([]byte(nil), nonce...)
	envelope = gcm.Seal(envelope, nonce, plaintext, []byte(weReadCredentialAAD))
	return s.repository.saveWeReadCredentialCiphertext(ctx, envelope, weReadCredentialVersion)
}

// loadCredentials 读取并解密微信读书凭据。
// 输入：ctx 控制查询。
// 输出：返回完整凭据；未绑定或密文无效时返回错误。
// 副作用：只读 PostgreSQL。
func (s *WeReadSource) loadCredentials(ctx context.Context) (client.WeReadArticleCredentials, error) {
	// 1. 读取固定版本密文并拆分 nonce 和认证密文。
	envelope, version, found, err := s.repository.loadWeReadCredentialCiphertext(ctx)
	if err != nil {
		return client.WeReadArticleCredentials{}, err
	}
	if !found {
		return client.WeReadArticleCredentials{}, fmt.Errorf("尚未绑定微信读书")
	}
	if version != weReadCredentialVersion {
		return client.WeReadArticleCredentials{}, fmt.Errorf("微信读书凭据版本 %d 不受支持", version)
	}
	gcm, err := s.credentialGCM()
	if err != nil {
		return client.WeReadArticleCredentials{}, err
	}
	if len(envelope) < gcm.NonceSize()+gcm.Overhead() {
		return client.WeReadArticleCredentials{}, fmt.Errorf("微信读书凭据密文长度不足")
	}
	nonce, ciphertext := envelope[:gcm.NonceSize()], envelope[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(weReadCredentialAAD))
	if err != nil {
		return client.WeReadArticleCredentials{}, fmt.Errorf("解密微信读书凭据: %w", err)
	}

	// 2. 严格解码唯一凭据结构并再次校验。
	var credentials client.WeReadArticleCredentials
	decoder := json.NewDecoder(strings.NewReader(string(plaintext)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		return client.WeReadArticleCredentials{}, fmt.Errorf("解析微信读书凭据: %w", err)
	}
	return credentials, credentials.Validate()
}

// credentialGCM 创建微信读书凭据使用的 AES-256-GCM。
// 输入：无。
// 输出：返回认证加密器。
// 副作用：无。
func (s *WeReadSource) credentialGCM() (cipher.AEAD, error) {
	// 1. 使用构造时派生的固定三十二字节密钥。
	block, err := aes.NewCipher(s.encryptionKey[:])
	if err != nil {
		return nil, fmt.Errorf("创建微信读书凭据密码块: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建微信读书凭据 GCM: %w", err)
	}
	return gcm, nil
}

// updateLoginFlow 更新仍为当前流程的公开状态。
// 输入：flow 是目标流程，state/message 是新状态，clearSecret 决定是否清除二维码秘密。
// 输出：无。
// 副作用：修改内存扫码状态。
func (s *WeReadSource) updateLoginFlow(flow *weReadLoginFlow, state, message string, clearSecret bool) {
	// 1. 只更新当前指针，避免旧请求覆盖新流程。
	s.flowMutex.Lock()
	defer s.flowMutex.Unlock()
	if s.flow != flow {
		return
	}
	flow.state, flow.message = state, message
	if clearSecret {
		flow.uuid, flow.confirmURL, flow.last = "", "", nil
	}
}

// publicWeReadLoginStatus 转换内存流程为可公开状态。
// 输入：flow 是当前流程。
// 输出：返回不包含 UUID 和确认地址的状态。
// 副作用：无。
func publicWeReadLoginStatus(flow *weReadLoginFlow) WeReadLoginStatus {
	// 1. 仅返回页面需要的状态、说明和有效期。
	return WeReadLoginStatus{State: flow.state, Message: flow.message, ExpiresAt: flow.expiresAt.Format(time.RFC3339)}
}

// dueWeReadAccounts 筛出当前已经到达抓取频率的公众号。
// 输入：accounts 是启用账号，now 是本次任务时间。
// 输出：返回需要访问微信读书的账号列表。
// 副作用：无。
func dueWeReadAccounts(accounts []WeReadAccount, now time.Time) []WeReadAccount {
	// 1. 未检查过或时间解析失败时允许抓取，避免永久卡死。
	due := make([]WeReadAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.LastCheckedAt == "" {
			due = append(due, account)
			continue
		}
		checkedAt, err := time.ParseInLocation("2006-01-02 15:04:05", account.LastCheckedAt, time.FixedZone("Asia/Shanghai", 8*60*60))
		if err != nil {
			due = append(due, account)
			continue
		}
		interval := time.Duration(account.FetchIntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = 12 * time.Hour
		}
		if !checkedAt.Add(interval).After(now) {
			due = append(due, account)
		}
	}
	return due
}

// weReadArticleKey 生成微信读书文章的稳定来源内主键。
// 输入：sourceID 是来源主键，reviewID 是微信读书文章标识。
// 输出：返回 SHA-256 十六进制摘要。
// 副作用：无。
func weReadArticleKey(sourceID int64, reviewID string) string {
	// 1. 来源和 reviewId 同时参与摘要，保持与通用文章键语义一致。
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d|%s", sourceID, reviewID)))
	return fmt.Sprintf("%x", hash[:])
}

// weReadArticleTitle 选择列表接口提供的稳定文章标题。
// 输入：listedTitle 来自公众号文章列表，detailTitle 来自详情接口。
// 输出：优先返回列表标题，列表缺失时才回退详情标题。
// 副作用：无。
func weReadArticleTitle(listedTitle, detailTitle string) string {
	// 1. 两个接口的 title 都可能混入正文，只保留首个非空行作为展示标题。
	for _, value := range []string{listedTitle, detailTitle} {
		for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
			if title := strings.TrimSpace(line); title != "" {
				return title
			}
		}
	}
	return ""
}

// weReadDetailContentFallback 从详情异常长标题中恢复被上游错放的正文。
// 输入：detailTitle 是微信读书详情接口的 title 字段。
// 输出：明显为长正文时返回规范文本，否则返回空字符串。
// 副作用：无。
func weReadDetailContentFallback(detailTitle string) string {
	// 1. 只接受超过两百字的长文本，避免把普通长标题误判为正文。
	content := strings.Join(strings.Fields(strings.TrimSpace(detailTitle)), " ")
	if len([]rune(content)) <= 200 {
		return ""
	}
	return content
}

// weReadPublishedAt 把 Unix 秒转换为数据库统一上海时间文本。
// 输入：timestamp 是文章发布时间秒。
// 输出：返回 YYYY-MM-DD HH:MM:SS，非法时间返回空字符串。
// 副作用：无。
func weReadPublishedAt(timestamp int64) string {
	// 1. 正时间转换到固定上海时区。
	if timestamp <= 0 {
		return ""
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return time.Unix(timestamp, 0).In(location).Format("2006-01-02 15:04:05")
}

// WeReadBinding 返回当前服务的微信读书绑定状态。
// 输入：ctx 控制数据库查询。
// 输出：返回连接和公众号状态。
// 副作用：只读 PostgreSQL 和内存。
func (s *Service) WeReadBinding(ctx context.Context) (WeReadBinding, error) {
	// 1. 要求当前进程已经装配微信读书来源。
	if s.options.WeRead == nil {
		return WeReadBinding{}, fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.Binding(ctx)
}

// StartWeReadLogin 创建微信读书扫码流程。
// 输入：ctx 控制上游请求。
// 输出：返回公开流程状态。
// 副作用：调用微信上游并修改内存。
func (s *Service) StartWeReadLogin(ctx context.Context) (WeReadLoginStatus, error) {
	// 1. 复用文章来源中的单流程管理器。
	if s.options.WeRead == nil {
		return WeReadLoginStatus{}, fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.StartLogin(ctx)
}

// PollWeReadLogin 推进微信读书扫码流程一次。
// 输入：ctx 控制上游和数据库操作。
// 输出：返回最新公开状态。
// 副作用：可能绑定微信读书并写 PostgreSQL。
func (s *Service) PollWeReadLogin(ctx context.Context) (WeReadLoginStatus, error) {
	// 1. 复用文章来源中的流程状态机。
	if s.options.WeRead == nil {
		return WeReadLoginStatus{}, fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.PollLogin(ctx)
}

// WeReadLoginQRPNG 返回当前微信读书二维码 PNG。
// 输入：无。
// 输出：返回 PNG 字节。
// 副作用：读取内存流程并编码图片。
func (s *Service) WeReadLoginQRPNG() ([]byte, error) {
	// 1. 复用文章来源二维码输出。
	if s.options.WeRead == nil {
		return nil, fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.LoginQRPNG()
}

// RefreshWeReadAccounts 重新发现微信读书书架公众号。
// 输入：ctx 控制请求。
// 输出：返回最新公众号列表。
// 副作用：调用微信上游并写 PostgreSQL。
func (s *Service) RefreshWeReadAccounts(ctx context.Context) ([]WeReadAccount, error) {
	// 1. 复用文章来源的书架发现入口。
	if s.options.WeRead == nil {
		return nil, fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.RefreshAccounts(ctx)
}

// SetWeReadAccountEnabled 更新公众号抓取开关。
// 输入：ctx 控制写入，accountID 和 enabled 描述目标公众号状态。
// 输出：返回更新错误。
// 副作用：写 PostgreSQL。
func (s *Service) SetWeReadAccountEnabled(ctx context.Context, accountID string, enabled bool) error {
	// 1. 复用文章来源的公众号设置入口。
	if s.options.WeRead == nil {
		return fmt.Errorf("微信读书文章来源未配置")
	}
	return s.options.WeRead.SetAccountEnabled(ctx, accountID, enabled)
}

// SetWeReadAccountSettings 更新公众号抓取频率和单次数量。
// 输入：ctx 控制写入，accountID 和 settings 描述目标策略。
// 输出：返回参数或写入错误。
// 副作用：写 PostgreSQL。
func (s *Service) SetWeReadAccountSettings(ctx context.Context, accountID string, settings WeReadAccountSettings) error {
	// 1. 校验范围，避免页面误操作导致高频访问。
	if s.options.WeRead == nil {
		return fmt.Errorf("微信读书文章来源未配置")
	}
	if !strings.HasPrefix(accountID, "MP_WXS_") {
		return fmt.Errorf("微信读书公众号 ID 无效")
	}
	if settings.FetchIntervalMinutes < 30 || settings.FetchIntervalMinutes > 10080 {
		return fmt.Errorf("获取频率必须在 30 分钟到 7 天之间")
	}
	if settings.FetchLimit < 1 || settings.FetchLimit > 50 {
		return fmt.Errorf("每次获取数量必须在 1 到 50 之间")
	}
	return s.options.WeRead.repository.updateWeReadAccountSettings(ctx, accountID, settings)
}
