package vpn

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// Service 统一处理 VPN 资源发现、订阅密钥和 Go 直连分发。
type Service struct {
	repository *Repository
	sources    *SourceCatalog
	publicURL  string
	tokenKey   []byte
}

// NewService 创建 VPN 订阅服务。
// 输入：repository 读写订阅，sources 读取私有资源，publicURL 提供公开根地址，secret 派生订阅密钥。
// 输出：返回 HTTP 页面可用服务。
// 副作用：无，不访问数据库、文件或外部接口。
func NewService(repository *Repository, sources *SourceCatalog, publicURL string, secret string) *Service {
	// 1. 使用独立上下文派生 VPN HMAC 密钥，避免直接复用原始应用密钥。
	derived := sha256.Sum256([]byte("aowugong:vpn-subscription:" + secret))
	return &Service{repository: repository, sources: sources, publicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"), tokenKey: derived[:]}
}

// Summary 返回当前登录用户可见的订阅及分发状态。
// 输入：ctx 是调用上下文，viewerID 是当前用户，canManage 表示是否为管理员。
// 输出：管理员获得全部用户订阅和可分配用户，普通用户只获得自己的订阅。
// 副作用：读取 PostgreSQL、VPN 私有目录和公共规则正文。
func (s *Service) Summary(ctx context.Context, viewerID int64, canManage bool) (Summary, error) {
	// 1. 读取资源以及当前身份允许查看的订阅。
	profiles, err := s.sources.Profiles()
	if err != nil {
		return Summary{}, fmt.Errorf("列出 VPN 资源: %w", err)
	}
	var storedSubscriptions []storedSubscription
	if canManage {
		storedSubscriptions, err = s.repository.List(ctx)
	} else {
		storedSubscriptions, err = s.repository.ListForUser(ctx, viewerID)
	}
	if err != nil {
		return Summary{}, fmt.Errorf("列出 VPN 订阅设备: %w", err)
	}
	subscriptions := make([]UserSubscription, 0, len(storedSubscriptions))
	for _, stored := range storedSubscriptions {
		subscriptions = append(subscriptions, s.publicSubscription(stored, profiles))
	}
	// 2. 所有有权查看 VPN 资源的用户都能看到同一份公共规则原文；管理员额外取得可开通用户。
	users := make([]UserOption, 0)
	content, routingErr := s.sources.CommonRouting()
	if routingErr != nil {
		return Summary{}, fmt.Errorf("读取公共 VPN 分流规则: %w", routingErr)
	}
	commonRouting := &CommonRouting{Filename: content.Filename, Body: content.Body}
	if canManage {
		users, err = s.repository.ListUsers(ctx)
		if err != nil {
			return Summary{}, fmt.Errorf("列出 VPN 可分配用户: %w", err)
		}
	} else {
		profiles = visibleProfiles(profiles, storedSubscriptions)
	}
	return Summary{
		DistributorConfigured: s.publicURL != "", DistributorURL: s.publicURL,
		CanManage: canManage, Profiles: profiles, Subscriptions: subscriptions, Users: users, CommonRouting: commonRouting,
	}, nil
}

// Create 新增订阅并发布首次订阅。
// 输入：ctx 是调用上下文，request 包含用户主键和资源编码。
// 输出：返回包含客户端订阅地址的订阅。
// 副作用：读取私有配置并写 PostgreSQL。
func (s *Service) Create(ctx context.Context, request CreateRequest) (UserSubscription, error) {
	// 1. 清理字段并确认资源当前可用。
	request.ProfileCode = strings.ToLower(strings.TrimSpace(request.ProfileCode))
	if request.UserID <= 0 || !validProfileCode(request.ProfileCode) {
		return UserSubscription{}, ErrInvalidInput
	}
	profiles, err := s.sources.Profiles()
	if err != nil {
		return UserSubscription{}, fmt.Errorf("检查 VPN 资源: %w", err)
	}
	if !containsProfile(profiles, request.ProfileCode) {
		return UserSubscription{}, ErrProfileNotFound
	}

	// 2. 创建不含明文 Token 的数据库记录；公开地址未配置时保留草稿供稍后发布。
	stored, err := s.repository.Create(ctx, request)
	if err != nil {
		return UserSubscription{}, err
	}
	if s.publicURL == "" {
		return s.publicSubscription(stored, profiles), nil
	}

	// 3. 公开地址可用时立即发布派生订阅并更新状态。
	if err := s.validatePublication(stored.ProfileCode); err != nil {
		return UserSubscription{}, s.publicationFailure(ctx, stored.ID, err)
	}
	stored, err = s.repository.MarkPublished(ctx, stored.ID)
	if err != nil {
		return UserSubscription{}, err
	}
	return s.publicSubscription(stored, profiles), nil
}

// Publish 校验当前版本的全部订阅格式并更新发布状态。
// 输入：ctx 是调用上下文，subscriptionID 是订阅主键。
// 输出：返回更新发布状态后的订阅。
// 副作用：读取私有配置并写 PostgreSQL。
func (s *Service) Publish(ctx context.Context, subscriptionID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 读取订阅并校验当前 Token 版本对应的配置。
	stored, err := s.repository.Get(ctx, subscriptionID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	if err := s.validatePublication(stored.ProfileCode); err != nil {
		return UserSubscription{}, s.publicationFailure(ctx, stored.ID, err)
	}
	stored, err = s.repository.MarkPublished(ctx, stored.ID)
	if err != nil {
		return UserSubscription{}, err
	}
	profiles, err := s.sources.Profiles()
	if err != nil {
		return UserSubscription{}, err
	}
	return s.publicSubscription(stored, profiles), nil
}

// Rotate 为订阅生成新订阅地址并撤销旧地址。
// 输入：ctx 是调用上下文，subscriptionID 是订阅主键。
// 输出：返回新版本订阅。
// 副作用：读写 PostgreSQL并读取私有配置。
func (s *Service) Rotate(ctx context.Context, subscriptionID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 先验证尚未写库的新版本，确保旧订阅在准备完成前继续可用。
	stored, err := s.repository.Get(ctx, subscriptionID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	newVersion := stored.TokenVersion + 1
	if err := s.validatePublication(stored.ProfileCode); err != nil {
		return UserSubscription{}, s.publicationFailure(ctx, stored.ID, err)
	}

	// 2. 提交新版本后，旧 Token 立即失效。
	stored, err = s.repository.UpdateTokenVersion(ctx, stored.ID, newVersion)
	if err != nil {
		return UserSubscription{}, err
	}
	stored, err = s.repository.MarkPublished(ctx, stored.ID)
	if err != nil {
		return UserSubscription{}, err
	}
	profiles, err := s.sources.Profiles()
	if err != nil {
		return UserSubscription{}, err
	}
	return s.publicSubscription(stored, profiles), nil
}

// Revoke 撤销订阅当前订阅并保留审计记录。
// 输入：ctx 是调用上下文，subscriptionID 是订阅主键。
// 输出：返回已撤销订阅。
// 副作用：写 PostgreSQL，使旧 Token 立即失效。
func (s *Service) Revoke(ctx context.Context, subscriptionID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 读取并鉴权，撤销状态统一阻断订阅。
	stored, err := s.repository.Get(ctx, subscriptionID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	stored, err = s.repository.MarkRevoked(ctx, stored.ID)
	if err != nil {
		return UserSubscription{}, err
	}
	profiles, err := s.sources.Profiles()
	if err != nil {
		return UserSubscription{}, err
	}
	return s.publicSubscription(stored, profiles), nil
}

// QRCode 生成指定客户端格式的订阅地址二维码 PNG。
// 输入：ctx 是调用上下文，subscriptionID 是订阅主键，format 是客户端格式。
// 输出：返回 320 像素二维码；格式不可用时返回 ErrFormatNotFound。
// 副作用：读取 PostgreSQL 和 VPN 私有目录文件名。
func (s *Service) QRCode(ctx context.Context, subscriptionID, viewerID int64, canManage bool, format string) ([]byte, error) {
	// 1. 读取当前订阅并确认格式属于其资源。
	stored, err := s.repository.Get(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if !canManage && stored.UserID != viewerID {
		return nil, ErrNotFound
	}
	profiles, err := s.sources.Profiles()
	if err != nil {
		return nil, err
	}
	subscription := s.publicSubscription(stored, profiles)
	subscriptionURL, exists := subscription.Subscriptions[strings.TrimSpace(format)]
	if !exists || subscriptionURL == "" || stored.Status == StatusRevoked {
		return nil, ErrFormatNotFound
	}

	// 2. v2rayNG 手机端直接编码单节点；多节点和其他客户端保留订阅地址。
	payload := subscriptionURL
	if format == "v2ray" {
		config, configErr := s.Subscription(ctx, stored.ID, s.deriveToken(stored.ID, stored.TokenVersion), format)
		if configErr != nil {
			return nil, configErr
		}
		payload = v2rayQRCodePayload(config.Body, subscriptionURL)
	}

	// 3. 只在内存生成二维码，不写入文件或日志。
	image, err := qrcode.Encode(payload, qrcode.Medium, 320)
	if err != nil {
		return nil, fmt.Errorf("生成 VPN 订阅二维码: %w", err)
	}
	return image, nil
}

// v2rayQRCodePayload 为手机二维码选择单节点分享链接。
// 输入：body 是 v2ray 订阅正文，fallback 是电脑端订阅地址。
// 输出：恰好一个受支持节点时返回分享链接，否则返回订阅地址。
// 副作用：无。
func v2rayQRCodePayload(body, fallback string) string {
	// 1. 优先解码标准 Base64 订阅，兼容已经是明文分享链接的正文。
	content := strings.TrimSpace(body)
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
		content = strings.TrimSpace(string(decoded))
	}

	// 2. 只有单节点才能由一个二维码完整表达，多节点继续使用订阅地址。
	links := make([]string, 0, 1)
	for _, line := range strings.Split(content, "\n") {
		if cleaned := strings.TrimSpace(line); cleaned != "" {
			links = append(links, cleaned)
		}
	}
	if len(links) != 1 {
		return fallback
	}
	for _, prefix := range []string{"vmess://", "vless://", "trojan://", "ss://", "hysteria2://", "hy2://", "tuic://"} {
		if strings.HasPrefix(strings.ToLower(links[0]), prefix) {
			return links[0]
		}
	}
	return fallback
}

// Subscription 校验订阅密钥并返回指定客户端订阅正文。
// 输入：ctx 是调用上下文，subscriptionID、token 和 format 来自公开订阅路径。
// 输出：返回对应格式配置；订阅、密钥或格式无效时统一返回 ErrNotFound。
// 副作用：读取 PostgreSQL 和 VPN 私有配置文件。
func (s *Service) Subscription(ctx context.Context, subscriptionID int64, token, format string) (ConfigContent, error) {
	// 1. 读取有效用户订阅并使用恒定时间比较当前版本密钥。
	stored, err := s.repository.Get(ctx, subscriptionID)
	if err != nil || stored.Status != StatusActive || stored.PublishedAt == nil {
		return ConfigContent{}, ErrNotFound
	}
	expected := s.deriveToken(stored.ID, stored.TokenVersion)
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(token))) {
		return ConfigContent{}, ErrNotFound
	}

	// 2. 实时构建当前私有资源，只返回路径指定格式。
	configs, err := s.sources.Build(stored.ProfileCode)
	if err != nil {
		return ConfigContent{}, fmt.Errorf("生成 VPN 公开订阅: %w", err)
	}
	config, exists := configs[strings.TrimSpace(format)]
	if !exists {
		return ConfigContent{}, ErrNotFound
	}
	return config, nil
}

// validatePublication 校验全部格式和公开根地址，订阅由 Go 实时提供。
func (s *Service) validatePublication(profileCode string) error {
	if _, err := s.sources.Build(profileCode); err != nil {
		return fmt.Errorf("生成 VPN 订阅配置: %w", err)
	}
	if s.publicURL == "" {
		return fmt.Errorf("发布 VPN 订阅配置: %w", ErrDistributorNotConfigured)
	}
	return nil
}

// publicationFailure 保留原始错误，同时报告失败状态无法写入的原因。
func (s *Service) publicationFailure(ctx context.Context, id int64, cause error) error {
	if _, err := s.repository.MarkPublishFailed(ctx, id, safePublishError(cause)); err != nil {
		return errors.Join(cause, fmt.Errorf("保存 VPN 发布失败状态: %w", err))
	}
	return cause
}

// publicSubscription 把内部记录转换为用户订阅响应。
// 输入：stored 是数据库记录，profiles 描述可用格式。
// 输出：返回可复制订阅地址，不包含派生密钥原料。
// 副作用：无。
func (s *Service) publicSubscription(stored storedSubscription, profiles []Profile) UserSubscription {
	// 1. 仅给未撤销订阅生成当前资源仍支持的订阅地址。
	subscriptions := make(map[string]string)
	if stored.Status != StatusRevoked && stored.PublishedAt != nil && s.publicURL != "" {
		token := s.deriveToken(stored.ID, stored.TokenVersion)
		for _, profile := range profiles {
			if profile.Code != stored.ProfileCode {
				continue
			}
			for _, format := range profile.Formats {
				subscriptions[format.Code] = s.publicURL + "/api/v1/vpn/subscriptions/" +
					strconv.FormatInt(stored.ID, 10) + "/" + url.PathEscape(token) + "/" + format.Code
			}
		}
	}
	return UserSubscription{
		ID: stored.ID, UserID: stored.UserID, Username: stored.Username, ProfileCode: stored.ProfileCode,
		TokenVersion: stored.TokenVersion, Status: stored.Status, PublishedAt: stored.PublishedAt,
		LastError: stored.LastError, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt,
		Subscriptions: subscriptions,
	}
}

// visibleProfiles 仅保留普通用户订阅实际使用的资源描述。
// 输入：profiles 是全部私有资源元数据，subscriptions 是当前用户订阅。
// 输出：返回不泄露其他资源名称的最小列表。
// 副作用：无。
func visibleProfiles(profiles []Profile, subscriptions []storedSubscription) []Profile {
	// 1. 没有订阅时返回空列表，有多条订阅时按资源编码去重筛选。
	if len(subscriptions) == 0 {
		return []Profile{}
	}
	assigned := make(map[string]struct{}, len(subscriptions))
	for _, subscription := range subscriptions {
		assigned[subscription.ProfileCode] = struct{}{}
	}
	visible := make([]Profile, 0, len(assigned))
	for _, profile := range profiles {
		if _, exists := assigned[profile.Code]; exists {
			visible = append(visible, profile)
		}
	}
	return visible
}

// deriveToken 按订阅和版本确定性派生高强度订阅密钥。
// 输入：subscriptionID 是数据库主键，version 是轮换版本。
// 输出：返回 256 位 URL 安全 Token。
// 副作用：无。
func (s *Service) deriveToken(subscriptionID int64, version int) string {
	// 1. HMAC 绑定订阅和版本；保留历史 device: 前缀以兼容已发放链接。
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write([]byte("device:" + strconv.FormatInt(subscriptionID, 10) + ":version:" + strconv.Itoa(version)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// containsProfile 判断资源列表是否包含目标编码。
// 输入：profiles 是资源列表，code 是目标编码。
// 输出：精确匹配时返回 true。
// 副作用：无。
func containsProfile(profiles []Profile, code string) bool {
	// 1. 二分查找已排序资源列表并复用同一个索引。
	index := sort.Search(len(profiles), func(index int) bool { return profiles[index].Code >= code })
	return index < len(profiles) && profiles[index].Code == code
}

// safePublishError 生成可持久化且不包含配置正文的发布错误。
// 输入：err 是发布链路错误。
// 输出：返回最多 500 个字符的错误摘要。
// 副作用：无。
func safePublishError(err error) string {
	// 1. 限制长度，错误链本身不包含 Token 或节点正文。
	message := err.Error()
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	return message
}
