package vpn

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// Service 统一处理 VPN 资源发现、设备密钥和 Go 直连分发。
type Service struct {
	repository  *Repository
	sources     *SourceCatalog
	distributor Distributor
	tokenKey    []byte
}

// NewService 创建 VPN 订阅服务。
// 输入：repository 读写设备，sources 读取私有资源，distributor 发布配置，secret 派生设备密钥。
// 输出：返回 HTTP 页面可用服务。
// 副作用：无，不访问数据库、文件或外部接口。
func NewService(repository *Repository, sources *SourceCatalog, distributor Distributor, secret string) *Service {
	// 1. 使用独立上下文派生 VPN HMAC 密钥，避免直接复用原始应用密钥。
	derived := sha256.Sum256([]byte("aowugong:vpn-subscription:" + secret))
	return &Service{repository: repository, sources: sources, distributor: distributor, tokenKey: derived[:]}
}

// Summary 返回当前登录用户可见的订阅及分发状态。
// 输入：ctx 是调用上下文，viewerID 是当前用户，canManage 表示是否为管理员。
// 输出：管理员获得全部用户订阅和可分配用户，普通用户只获得自己的订阅。
// 副作用：读取 PostgreSQL 和 VPN 私有目录文件名。
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
	// 2. 管理员额外取得可开通用户；普通用户不接触私有资源目录清单。
	users := make([]UserOption, 0)
	if canManage {
		users, err = s.repository.ListUsers(ctx)
		if err != nil {
			return Summary{}, fmt.Errorf("列出 VPN 可分配用户: %w", err)
		}
	} else {
		profiles = visibleProfiles(profiles, storedSubscriptions)
	}
	return Summary{
		DistributorConfigured: s.distributor.Configured(), DistributorURL: s.distributor.BaseURL(),
		CanManage: canManage, Profiles: profiles, Subscriptions: subscriptions, Users: users,
	}, nil
}

// Create 新增设备并发布首次订阅。
// 输入：ctx 是调用上下文，request 包含设备名和资源编码。
// 输出：返回包含客户端订阅地址的设备。
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

	// 2. 创建不含明文 Token 的数据库记录；分发器未配置时保留草稿供稍后发布。
	stored, err := s.repository.Create(ctx, request)
	if err != nil {
		return UserSubscription{}, err
	}
	if !s.distributor.Configured() {
		return s.publicSubscription(stored, profiles), nil
	}

	// 3. 分发器可用时立即发布派生订阅并更新状态。
	if err := s.publishStored(ctx, stored); err != nil {
		_, _ = s.repository.MarkPublishFailed(ctx, stored.ID, safePublishError(err))
		return UserSubscription{}, err
	}
	stored, err = s.repository.MarkPublished(ctx, stored.ID)
	if err != nil {
		return UserSubscription{}, err
	}
	return s.publicSubscription(stored, profiles), nil
}

// Publish 重新推送设备当前版本的全部订阅格式。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回更新发布状态后的设备。
// 副作用：读取私有配置并写 PostgreSQL。
func (s *Service) Publish(ctx context.Context, deviceID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 读取设备并使用当前 Token 版本覆盖远端配置。
	stored, err := s.repository.Get(ctx, deviceID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	if err := s.publishStored(ctx, stored); err != nil {
		_, _ = s.repository.MarkPublishFailed(ctx, stored.ID, safePublishError(err))
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

// Rotate 为设备生成新订阅地址并撤销旧地址。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回新版本设备。
// 副作用：读写 PostgreSQL并读取私有配置。
func (s *Service) Rotate(ctx context.Context, deviceID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 先发布尚未写库的新版本，确保旧订阅在准备完成前继续可用。
	stored, err := s.repository.Get(ctx, deviceID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	oldHash := hashToken(s.deriveToken(stored.ID, stored.TokenVersion))
	newVersion := stored.TokenVersion + 1
	prospective := stored
	prospective.TokenVersion = newVersion
	if err := s.publishStored(ctx, prospective); err != nil {
		_, _ = s.repository.MarkPublishFailed(ctx, stored.ID, safePublishError(err))
		return UserSubscription{}, err
	}

	// 2. 提交新版本后删除旧 KV；旧值删除失败不回滚可用的新订阅。
	stored, err = s.repository.UpdateTokenVersion(ctx, stored.ID, newVersion)
	if err != nil {
		_ = s.distributor.Revoke(ctx, hashToken(s.deriveToken(prospective.ID, prospective.TokenVersion)))
		return UserSubscription{}, err
	}
	if err := s.distributor.Revoke(ctx, oldHash); err != nil {
		_, _ = s.repository.MarkPublishFailed(ctx, stored.ID, safePublishError(err))
		return UserSubscription{}, fmt.Errorf("新订阅已生效但撤销旧订阅失败: %w", err)
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

// Revoke 撤销设备当前订阅并保留审计记录。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回已撤销设备。
// 副作用：写 PostgreSQL，使旧 Token 立即失效。
func (s *Service) Revoke(ctx context.Context, deviceID, viewerID int64, canManage bool) (UserSubscription, error) {
	// 1. 草稿从未发布到远端，直接更新本地状态。
	stored, err := s.repository.Get(ctx, deviceID)
	if err != nil {
		return UserSubscription{}, err
	}
	if !canManage && stored.UserID != viewerID {
		return UserSubscription{}, ErrNotFound
	}
	if stored.PublishedAt != nil {
		// 2. 已发布设备先删除远端订阅，成功后再更新本地状态。
		if err := s.distributor.Revoke(ctx, hashToken(s.deriveToken(stored.ID, stored.TokenVersion))); err != nil {
			return UserSubscription{}, fmt.Errorf("撤销 VPN 远端订阅: %w", err)
		}
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

// QRCode 生成设备某格式订阅地址的二维码 PNG。
// 输入：ctx 是调用上下文，deviceID 是设备主键，format 是客户端格式。
// 输出：返回 320 像素二维码；格式不可用时返回 ErrFormatNotFound。
// 副作用：读取 PostgreSQL 和 VPN 私有目录文件名。
func (s *Service) QRCode(ctx context.Context, deviceID, viewerID int64, canManage bool, format string) ([]byte, error) {
	// 1. 读取当前设备并确认格式属于其资源。
	stored, err := s.repository.Get(ctx, deviceID)
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
	device := s.publicSubscription(stored, profiles)
	subscriptionURL, exists := device.Subscriptions[strings.TrimSpace(format)]
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

// Subscription 校验设备密钥并返回指定客户端订阅正文。
// 输入：ctx 是调用上下文，deviceID、token 和 format 来自公开订阅路径。
// 输出：返回对应格式配置；设备、密钥或格式无效时统一返回 ErrNotFound。
// 副作用：读取 PostgreSQL 和 VPN 私有配置文件。
func (s *Service) Subscription(ctx context.Context, deviceID int64, token, format string) (ConfigContent, error) {
	// 1. 读取有效用户订阅并使用恒定时间比较当前版本密钥。
	stored, err := s.repository.Get(ctx, deviceID)
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

// publishStored 构建并推送一台数据库设备的当前版本。
// 输入：ctx 是调用上下文，stored 是设备内部记录。
// 输出：发布成功返回 nil。
// 副作用：读取私有配置。
func (s *Service) publishStored(ctx context.Context, stored storedSubscription) error {
	// 1. 构建全部格式并使用 Token 哈希作为远端 KV 键。
	configs, err := s.sources.Build(stored.ProfileCode)
	if err != nil {
		return fmt.Errorf("生成 VPN 订阅配置: %w", err)
	}
	token := s.deriveToken(stored.ID, stored.TokenVersion)
	payload := DistributionPayload{
		User: stored.Username, Profile: stored.ProfileCode, Formats: configs,
		IssuedAt: time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format(time.RFC3339),
	}
	if err := s.distributor.Publish(ctx, hashToken(token), payload); err != nil {
		return fmt.Errorf("发布 VPN 订阅配置: %w", err)
	}
	return nil
}

// publicSubscription 把内部记录转换为用户订阅响应。
// 输入：stored 是数据库记录，profiles 描述可用格式。
// 输出：返回可复制订阅地址，不包含派生密钥原料。
// 副作用：无。
func (s *Service) publicSubscription(stored storedSubscription, profiles []Profile) UserSubscription {
	// 1. 仅给未撤销设备生成当前资源仍支持的订阅地址。
	subscriptions := make(map[string]string)
	if stored.Status != StatusRevoked && stored.PublishedAt != nil && s.distributor.BaseURL() != "" {
		token := s.deriveToken(stored.ID, stored.TokenVersion)
		for _, profile := range profiles {
			if profile.Code != stored.ProfileCode {
				continue
			}
			for _, format := range profile.Formats {
				subscriptions[format.Code] = s.distributor.BaseURL() + "/api/v1/vpn/subscriptions/" +
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
	// 1. 没有订阅时返回空列表，有订阅时按资源编码筛选。
	if len(subscriptions) == 0 {
		return []Profile{}
	}
	visible := make([]Profile, 0, 1)
	for _, profile := range profiles {
		if profile.Code == subscriptions[0].ProfileCode {
			visible = append(visible, profile)
		}
	}
	return visible
}

// deriveToken 按设备和版本确定性派生高强度订阅密钥。
// 输入：deviceID 是数据库主键，version 是轮换版本。
// 输出：返回 256 位 URL 安全 Token。
// 副作用：无。
func (s *Service) deriveToken(deviceID int64, version int) string {
	// 1. HMAC 同时绑定业务域、设备和版本，数据库无需保存明文密钥。
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write([]byte("device:" + strconv.FormatInt(deviceID, 10) + ":version:" + strconv.Itoa(version)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// hashToken 计算统一分发接口使用的固定长度 Token 哈希。
// 输入：token 是 URL 中的订阅密钥。
// 输出：返回小写十六进制 SHA-256。
// 副作用：无。
func hashToken(token string) string {
	// 1. 远端只按哈希索引配置，不存储原始 URL 密钥。
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash[:])
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
