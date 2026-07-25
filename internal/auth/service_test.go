package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

const migratedBcryptPassword = "$2a$10$NY3688Y4F/CXkeYA3q118eMu7YCjEGKPaPGAFXZp9SewmzmAfE9Jy"

// TestServiceLoginAcceptsMigratedBcrypt 验证既有 60 字符 bcrypt 密码可以登录。
// 输入：迁移格式的 bcrypt 用户记录和正确密码。
// 输出：登录成功并返回令牌。
// 副作用：创建并写入临时 SQLite。
func TestServiceLoginAcceptsMigratedBcrypt(t *testing.T) {
	// 1. 写入模拟生产迁移后的 bcrypt 用户。
	service, db := newTestService(t)
	insertTestUser(t, db, "legacy", "legacy@example.com", migratedBcryptPassword, true)

	// 2. 使用原始密码登录并断言返回 Bearer 令牌。
	response, err := service.Login(context.Background(), LoginRequest{Username: "legacy", Password: "password"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if response.TokenType != "bearer" || response.AccessToken == "" {
		t.Errorf("Login() = %#v, want bearer token", response)
	}
}

// TestServiceLoginRejectsInactiveUser 验证禁用用户无法获取令牌。
// 输入：禁用用户和正确密码。
// 输出：登录返回 ErrUnauthorized。
// 副作用：创建并写入临时 SQLite。
func TestServiceLoginRejectsInactiveUser(t *testing.T) {
	// 1. 写入禁用用户。
	service, db := newTestService(t)
	insertTestUser(t, db, "disabled", "disabled@example.com", migratedBcryptPassword, false)

	// 2. 登录必须返回未授权业务错误。
	_, err := service.Login(context.Background(), LoginRequest{Username: "disabled", Password: "password"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Login() error = %v, want ErrUnauthorized", err)
	}
}

// TestServiceRegisterRejectsDuplicateUsername 验证注册拒绝重复用户名。
// 输入：已存在用户名和新的注册请求。
// 输出：注册返回 ErrConflict。
// 副作用：创建并写入临时 SQLite。
func TestServiceRegisterRejectsDuplicateUsername(t *testing.T) {
	// 1. 先注册一个用户。
	service, _ := newTestService(t)
	if _, err := service.Register(context.Background(), RegisterRequest{Username: "duplicate", Password: "password"}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	// 2. 相同用户名再次注册必须冲突。
	_, err := service.Register(context.Background(), RegisterRequest{Username: "duplicate", Password: "password"})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("second Register() error = %v, want ErrConflict", err)
	}
}

// TestServiceRegisterStoresBcryptPassword 验证新注册密码以 bcrypt 形式存储。
// 输入：新用户名和原始密码。
// 输出：数据库保存可验证且不等于明文的 bcrypt 哈希。
// 副作用：创建并写入临时 SQLite。
func TestServiceRegisterStoresBcryptPassword(t *testing.T) {
	// 1. 注册新的公开用户。
	service, db := newTestService(t)
	if _, err := service.Register(context.Background(), RegisterRequest{Username: "new-user", Password: "password"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 2. 密码字段必须是长度 60 的 bcrypt 哈希。
	var passwordHash string
	if err := db.QueryRow(`SELECT password FROM aowugong_fastapi_users WHERE username = ?`, "new-user").Scan(&passwordHash); err != nil {
		t.Fatalf("query password error = %v", err)
	}
	if len(passwordHash) != 60 {
		t.Errorf("bcrypt hash length = %d, want 60", len(passwordHash))
	}
}

// TestServiceProfileIncludesRolesAndPermissions 验证资料返回角色和权限集合。
// 输入：带 investor 角色的测试用户。
// 输出：资料包含用户、角色和对应页面权限。
// 副作用：创建并写入临时 SQLite。
func TestServiceProfileIncludesRolesAndPermissions(t *testing.T) {
	// 1. 写入带角色和权限关联的用户。
	service, db := newTestService(t)
	insertTestUser(t, db, "profile-user", "profile@example.com", migratedBcryptPassword, true)
	statements := []string{
		`INSERT INTO aowugong_roles (code, name, description, is_active, is_system) VALUES ('reader', 'Reader', '', 1, 0)`,
		"INSERT INTO aowugong_permissions (code, name, `group`, description) VALUES ('page:reader', 'Reader', 'test', '')",
		`INSERT INTO aowugong_user_roles (user_id, role_id) SELECT id, (SELECT id FROM aowugong_roles WHERE code = 'reader') FROM aowugong_fastapi_users WHERE username = 'profile-user'`,
		`INSERT INTO aowugong_role_permissions (role_id, permission_id) SELECT (SELECT id FROM aowugong_roles WHERE code = 'reader'), id FROM aowugong_permissions WHERE code = 'page:reader'`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("insert profile RBAC data error = %v", err)
		}
	}

	// 2. 查询资料必须返回关联角色和权限。
	profile, err := service.Profile(context.Background(), 1)
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != "reader" || len(profile.Permissions) != 1 || profile.Permissions[0] != "page:reader" {
		t.Errorf("Profile() = %#v, want reader role and page:reader permission", profile)
	}
}

// TestTokenManagerUsesExactlySeventyTwoHours 验证 JWT 过期时间精确为 72 小时。
// 输入：固定签发时间和 72 小时令牌管理器。
// 输出：解析声明的有效期精确等于 72 小时。
// 副作用：无。
func TestTokenManagerUsesExactlySeventyTwoHours(t *testing.T) {
	// 1. 在固定时刻签发令牌。
	issuedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager("test-secret", 72*time.Hour)
	token, err := manager.Create("alice", issuedAt)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 2. 解析后过期时间必须恰好相差 72 小时。
	claims, err := manager.Parse(token, issuedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := claims.ExpiresAt.Sub(claims.IssuedAt); got != 72*time.Hour {
		t.Errorf("expiry = %s, want 72h", got)
	}
}

// TestTokenManagerRejectsMalformedAndExpiredToken 验证令牌解析拒绝非法和过期令牌。
// 输入：损坏令牌和超过到期时间的有效令牌。
// 输出：两种解析均返回 ErrInvalidToken。
// 副作用：无。
func TestTokenManagerRejectsMalformedAndExpiredToken(t *testing.T) {
	// 1. 创建可控时间的令牌管理器。
	issuedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager("test-secret", 72*time.Hour)
	token, err := manager.Create("alice", issuedAt)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 2. 非法和过期令牌均必须被拒绝。
	if _, err := manager.Parse("invalid.token", issuedAt); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse(malformed) error = %v, want ErrInvalidToken", err)
	}
	if _, err := manager.Parse(token, issuedAt.Add(72*time.Hour)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse(expired) error = %v, want ErrInvalidToken", err)
	}
}

// newTestService 创建使用隔离 SQLite 数据库的认证服务。
// 输入：t 管理临时数据库生命周期。
// 输出：返回认证服务和底层 SQLite 连接。
// 副作用：创建、迁移并注册清理临时 SQLite。
func newTestService(t *testing.T) (*Service, *sql.DB) {
	// 1. 打开并迁移独立的 SQLite 测试 schema。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 组装认证仓储、令牌管理器和服务。
	return NewService(NewRepository(db), NewTokenManager("test-secret", 72*time.Hour)), db
}

// insertTestUser 写入供认证场景使用的用户记录。
// 输入：测试句柄、数据库、用户字段、密码哈希和启用状态。
// 输出：无；写入失败时终止测试。
// 副作用：向临时 SQLite 插入用户。
func insertTestUser(t *testing.T, db *sql.DB, username, email, passwordHash string, active bool) {
	// 1. 直接写入迁移后表结构以模拟既有生产数据。
	t.Helper()
	if _, err := db.Exec(`INSERT INTO aowugong_fastapi_users (username, email, password, is_active) VALUES (?, ?, ?, ?)`, username, email, passwordHash, active); err != nil {
		t.Fatalf("insert user error = %v", err)
	}
}
