package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager 管理 HS256 JWT 的签发和校验。
type TokenManager struct {
	secret   []byte
	lifetime time.Duration
}

// NewTokenManager 创建使用指定密钥与有效期的令牌管理器。
func NewTokenManager(secret string, lifetime time.Duration) *TokenManager {
	// 1. 复制密钥并保留调用方指定的精确有效期。
	return &TokenManager{secret: []byte(secret), lifetime: lifetime}
}

// Create 在指定时刻为用户名签发 JWT。
func (m *TokenManager) Create(username string, issuedAt time.Time) (string, error) {
	// 1. 校验签发所需的最小输入。
	if username == "" || len(m.secret) == 0 || m.lifetime <= 0 {
		return "", ErrInvalidInput
	}

	// 2. 固定 UTC 时间并使用 HS256 写入 sub、iat、exp 声明。
	issuedAt = issuedAt.UTC()
	claims := jwt.RegisteredClaims{
		Subject:   username,
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(issuedAt.Add(m.lifetime)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("签发 JWT: %w", err)
	}
	return token, nil
}

// Parse 在指定时刻校验并解析 JWT。
func (m *TokenManager) Parse(token string, now time.Time) (Claims, error) {
	// 1. 解析令牌并限制只接受 HS256 签名。
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(parsed *jwt.Token) (any, error) {
		if parsed.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("JWT 签名算法不受支持: %w", ErrInvalidToken)
		}
		return m.secret, nil
	}, jwt.WithTimeFunc(func() time.Time { return now.UTC() }))
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return Claims{}, fmt.Errorf("解析 JWT: %w", ErrInvalidToken)
	}

	// 2. 返回经过库校验的身份和精确时间声明。
	return Claims{Username: claims.Subject, IssuedAt: claims.IssuedAt.Time, ExpiresAt: claims.ExpiresAt.Time}, nil
}
