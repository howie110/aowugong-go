// Package vpn 提供私有 VPN 资源转换、用户订阅和 Go 直连分发能力。
package vpn

import "errors"

const (
	StatusDraft   = "draft"
	StatusActive  = "active"
	StatusError   = "error"
	StatusRevoked = "revoked"
)

var (
	ErrInvalidInput             = errors.New("VPN 订阅参数无效")
	ErrNotFound                 = errors.New("VPN 订阅设备不存在")
	ErrConflict                 = errors.New("VPN 订阅设备名称已存在")
	ErrProfileNotFound          = errors.New("VPN 资源不存在")
	ErrFormatNotFound           = errors.New("VPN 订阅格式不存在")
	ErrUserNotFound             = errors.New("VPN 订阅用户不存在")
	ErrDistributorNotConfigured = errors.New("VPN 分发服务尚未配置")
)

// Format 描述一个客户端可消费的订阅格式。
type Format struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Profile 描述从私有目录发现的一组同源 VPN 配置。
type Profile struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Formats []Format `json:"formats"`
}

// UserSubscription 描述一名登录用户独享的 VPN 订阅。
type UserSubscription struct {
	ID            int64             `json:"id"`
	UserID        int64             `json:"user_id"`
	Username      string            `json:"username"`
	ProfileCode   string            `json:"profile_code"`
	TokenVersion  int               `json:"token_version"`
	Status        string            `json:"status"`
	PublishedAt   *string           `json:"published_at"`
	LastError     string            `json:"last_error"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
	Subscriptions map[string]string `json:"subscriptions"`
}

// CreateRequest 描述给登录用户开通订阅需要的最小字段。
type CreateRequest struct {
	UserID      int64  `json:"user_id"`
	ProfileCode string `json:"profile_code"`
}

// UserOption 描述管理员可选择的活动登录用户。
type UserOption struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	HasSubscription bool   `json:"has_subscription"`
}

// Summary 描述 VPN 页面一次加载所需的完整状态。
type Summary struct {
	DistributorConfigured bool               `json:"distributor_configured"`
	DistributorURL        string             `json:"distributor_url"`
	CanManage             bool               `json:"can_manage"`
	Profiles              []Profile          `json:"profiles"`
	Subscriptions         []UserSubscription `json:"user_subscriptions"`
	Users                 []UserOption       `json:"users"`
}

// ConfigContent 描述公开订阅接口返回的一份客户端配置。
type ConfigContent struct {
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
	Body        string `json:"body"`
}

// DistributionPayload 描述统一发布流程校验的完整用户订阅。
type DistributionPayload struct {
	User     string                   `json:"user"`
	Profile  string                   `json:"profile"`
	Formats  map[string]ConfigContent `json:"formats"`
	IssuedAt string                   `json:"issued_at"`
}

type storedSubscription struct {
	ID           int64
	UserID       int64
	Username     string
	ProfileCode  string
	TokenVersion int
	Status       string
	PublishedAt  *string
	LastError    string
	CreatedAt    string
	UpdatedAt    string
}
