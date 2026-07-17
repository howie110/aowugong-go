package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

const migratedBcryptPassword = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// TestServiceLoginAcceptsMigratedBcrypt 验证既有 60 字符 bcrypt 密码可以登录。
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
func TestServiceProfileIncludesRolesAndPermissions(t *testing.T) {
	// 1. 写入带角色和权限关联的用户。
	service, db := newTestService(t)
	insertTestUser(t, db, "profile-user", "profile@example.com", migratedBcryptPassword, true)
	if _, err := db.Exec(`
		INSERT INTO aowugong_roles (code, name, description, is_active, is_system) VALUES ('reader', 'Reader', '', 1, 0);
		INSERT INTO aowugong_permissions (code, name, "group", description) VALUES ('page:reader', 'Reader', 'test', '');
		INSERT INTO aowugong_user_roles (user_id, role_id) SELECT id, (SELECT id FROM aowugong_roles WHERE code = 'reader') FROM aowugong_fastapi_users WHERE username = 'profile-user';
		INSERT INTO aowugong_role_permissions (role_id, permission_id) SELECT (SELECT id FROM aowugong_roles WHERE code = 'reader'), id FROM aowugong_permissions WHERE code = 'page:reader';
	`); err != nil {
		t.Fatalf("insert profile RBAC data error = %v", err)
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

// newTestService 创建使用临时 SQLite 数据库的认证服务。
func newTestService(t *testing.T) (*Service, *sql.DB) {
	// 1. 打开并迁移独立的临时数据库。
	t.Helper()
	db, err := database.OpenSQLite(context.Background(), config.Database{Path: filepath.Join(t.TempDir(), "auth.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// 2. 组装认证仓储、令牌管理器和服务。
	return NewService(NewRepository(db), NewTokenManager("test-secret", 72*time.Hour)), db
}

// insertTestUser 写入供认证场景使用的用户记录。
func insertTestUser(t *testing.T, db *sql.DB, username, email, passwordHash string, active bool) {
	// 1. 直接写入迁移后表结构以模拟既有生产数据。
	t.Helper()
	if _, err := db.Exec(`INSERT INTO aowugong_fastapi_users (username, email, password, is_active) VALUES (?, ?, ?, ?)`, username, email, passwordHash, active); err != nil {
		t.Fatalf("insert user error = %v", err)
	}
}
