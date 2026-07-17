// Package auth 提供认证、用户与访问控制身份模型。
package auth

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized  = errors.New("未授权")
	ErrConflict      = errors.New("资源冲突")
	ErrNotFound      = errors.New("资源不存在")
	ErrInvalidToken  = errors.New("无效令牌")
	ErrInvalidInput  = errors.New("请求参数无效")
	ErrAccessDenied  = errors.New("无权访问")
)

// User 表示不暴露密码的用户记录。
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	IsActive  bool   `json:"is_active"`
	IsSuperuser bool `json:"-"`
}

// userRecord 表示包含密码哈希的内部用户记录。
type userRecord struct {
	User
	PasswordHash string
}

// LoginRequest 表示登录表单内容。
type LoginRequest struct {
	Username string
	Password string
}

// RegisterRequest 表示注册请求内容。
type RegisterRequest struct {
	Username string
	Password string
}

// CreateUserRequest 表示公开用户创建请求内容。
type CreateUserRequest struct {
	Username string
	Email    string
	Password string
}

// TokenResponse 表示 Bearer 令牌响应。
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// Profile 表示带角色和权限的当前用户资料。
type Profile struct {
	User
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// Claims 表示认证中使用的最小 JWT 声明。
type Claims struct {
	Username  string
	IssuedAt  time.Time
	ExpiresAt time.Time
}
