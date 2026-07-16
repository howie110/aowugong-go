package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service 统一处理登录、注册、令牌认证和当前用户资料。
type Service struct {
	repository *Repository
	tokens     *TokenManager
	now        func() time.Time
}

// NewService 创建认证服务。
// 输入：repository 是认证仓储，tokens 是 JWT 管理器。
// 输出：返回使用系统时钟的认证服务。
// 副作用：无。
func NewService(repository *Repository, tokens *TokenManager) *Service {
	// 1. 保存依赖并集中注入当前时间来源。
	return &Service{repository: repository, tokens: tokens, now: time.Now}
}

// Login 校验用户名、启用状态和 bcrypt 密码并签发令牌。
// 输入：ctx 是调用上下文，req 是用户名和明文密码。
// 输出：返回有效期由 TokenManager 固定的 Bearer 令牌。
// 副作用：读取 MySQL。
func (s *Service) Login(ctx context.Context, req LoginRequest) (TokenResponse, error) {
	// 1. 清理用户名并拒绝缺失凭据。
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return TokenResponse{}, ErrInvalidInput
	}

	// 2. 查询用户并统一隐藏用户名、状态和密码差异。
	record, err := s.repository.FindByUsername(ctx, req.Username)
	if err != nil {
		if err == ErrNotFound {
			return TokenResponse{}, ErrUnauthorized
		}
		return TokenResponse{}, fmt.Errorf("登录查询用户: %w", err)
	}
	if !record.IsActive {
		return TokenResponse{}, ErrUnauthorized
	}
	matched, err := VerifyPassword(req.Password, record.PasswordHash)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("登录校验密码: %w", err)
	}
	if !matched {
		return TokenResponse{}, ErrUnauthorized
	}

	// 3. 为验证通过的用户名签发 Bearer 令牌。
	token, err := s.tokens.Create(record.Username, s.now())
	if err != nil {
		return TokenResponse{}, fmt.Errorf("登录签发令牌: %w", err)
	}
	return TokenResponse{AccessToken: token, TokenType: "bearer"}, nil
}

// Register 创建公开注册用户并立即签发令牌。
// 输入：ctx 是调用上下文，req 是用户名和明文密码。
// 输出：返回新用户的 Bearer 令牌；用户名冲突时返回 ErrConflict。
// 副作用：写入 MySQL。
func (s *Service) Register(ctx context.Context, req RegisterRequest) (TokenResponse, error) {
	// 1. 校验用户名和密码并生成与旧项目兼容的默认邮箱。
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return TokenResponse{}, ErrInvalidInput
	}
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("注册生成密码哈希: %w", err)
	}

	// 2. 持久化新用户，唯一键冲突由仓储转换为业务错误。
	user, err := s.repository.Create(ctx, CreateUserRequest{
		Username: username,
		Email:    username + "@example.com",
		Password: req.Password,
	}, passwordHash)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("注册创建用户: %w", err)
	}

	// 3. 为新用户签发与登录一致的令牌。
	token, err := s.tokens.Create(user.Username, s.now())
	if err != nil {
		return TokenResponse{}, fmt.Errorf("注册签发令牌: %w", err)
	}
	return TokenResponse{AccessToken: token, TokenType: "bearer"}, nil
}

// Authenticate 校验 Bearer 令牌并返回当前活动用户。
// 输入：ctx 是调用上下文，token 是不带 Bearer 前缀的 JWT。
// 输出：返回认证用户；令牌无效或用户停用时返回 ErrUnauthorized。
// 副作用：读取 MySQL。
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	// 1. 校验令牌并读取其用户名声明。
	claims, err := s.tokens.Parse(token, s.now())
	if err != nil {
		return User{}, ErrUnauthorized
	}

	// 2. 读取当前数据库状态，避免已停用用户继续使用旧令牌。
	record, err := s.repository.FindByUsername(ctx, claims.Username)
	if err != nil || !record.IsActive {
		return User{}, ErrUnauthorized
	}
	return record.User, nil
}

// Profile 返回当前用户以及其角色和页面权限。
// 输入：ctx 是调用上下文，userID 是已认证用户主键。
// 输出：返回资料结构；失败时返回带业务上下文的错误。
// 副作用：读取 MySQL。
func (s *Service) Profile(ctx context.Context, userID int64) (Profile, error) {
	// 1. 通过唯一仓储入口组装用户、角色和权限。
	profile, err := s.repository.Profile(ctx, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("读取当前用户资料: %w", err)
	}
	return profile, nil
}
