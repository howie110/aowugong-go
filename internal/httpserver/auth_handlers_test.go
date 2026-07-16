package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestAuthLoginAndProfile 验证 OAuth2 表单登录后可以读取当前用户资料。
func TestAuthLoginAndProfile(t *testing.T) {
	// 1. 创建管理员用户并通过登录接口取得 Bearer 令牌。
	handler, db := newAuthenticatedTestRouter(t)
	createHTTPTestUser(t, db, "admin", "admin@example.com", "password", rbac.AdminRoleCode)
	token := loginHTTPTestUser(t, handler, "admin", "password")

	// 2. 使用令牌读取资料并断言角色和权限已展开。
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/profile", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("profile status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var profile auth.Profile
	if err := json.Unmarshal(recorder.Body.Bytes(), &profile); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if profile.Username != "admin" || len(profile.Roles) != 1 || profile.Roles[0] != rbac.AdminRoleCode {
		t.Errorf("profile = %#v, want admin role", profile)
	}
	if len(profile.Permissions) != len(rbac.DefaultPermissions) {
		t.Errorf("permission count = %d, want %d", len(profile.Permissions), len(rbac.DefaultPermissions))
	}
}

// TestAuthProfileRejectsMissingBearer 验证资料接口拒绝缺失的 Bearer 令牌。
func TestAuthProfileRejectsMissingBearer(t *testing.T) {
	// 1. 创建具备认证依赖的路由器但不发送令牌。
	handler, _ := newAuthenticatedTestRouter(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/profile", nil))

	// 2. 断言接口返回统一 401 错误信封。
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	assertErrorEnvelope(t, recorder, "unauthorized")
}

// TestPermissionsRequireAdminPermission 验证 investor 被拒绝而 admin 可以读取用户列表。
func TestPermissionsRequireAdminPermission(t *testing.T) {
	// 1. 创建两个角色用户并分别登录。
	handler, db := newAuthenticatedTestRouter(t)
	createHTTPTestUser(t, db, "admin", "admin@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "investor", "investor@example.com", "password", rbac.InvestorRoleCode)
	adminToken := loginHTTPTestUser(t, handler, "admin", "password")
	investorToken := loginHTTPTestUser(t, handler, "investor", "password")

	// 2. 投资者访问权限用户列表必须得到 403。
	investorRequest := httptest.NewRequest(http.MethodGet, "/api/v1/permissions/users", nil)
	investorRequest.Header.Set("Authorization", "Bearer "+investorToken)
	investorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(investorRecorder, investorRequest)
	if investorRecorder.Code != http.StatusForbidden {
		t.Errorf("investor status = %d, want %d", investorRecorder.Code, http.StatusForbidden)
	}

	// 3. 管理员访问同一路径必须获得两个用户。
	adminRequest := httptest.NewRequest(http.MethodGet, "/api/v1/permissions/users", nil)
	adminRequest.Header.Set("Authorization", "Bearer "+adminToken)
	adminRecorder := httptest.NewRecorder()
	handler.ServeHTTP(adminRecorder, adminRequest)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body = %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var users []rbac.UserRoles
	if err := json.Unmarshal(adminRecorder.Body.Bytes(), &users); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(users) != 2 {
		t.Errorf("user count = %d, want 2", len(users))
	}
}

// newAuthenticatedTestRouter 创建完成迁移和 RBAC 同步的 HTTP 测试路由器。
func newAuthenticatedTestRouter(t *testing.T) (http.Handler, *sql.DB) {
	// 1. 创建隔离 MySQL schema 并执行全部版本化迁移。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 组装并同步认证和 RBAC 服务。
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("http-test-secret", 72*time.Hour))
	return NewRouter(Dependencies{Auth: authService, RBAC: rbacService}), db
}

// createHTTPTestUser 创建 HTTP 场景使用的 bcrypt 用户并分配角色。
func createHTTPTestUser(t *testing.T, db *sql.DB, username, email, password, roleCode string) {
	// 1. 生成 bcrypt 密码并写入测试用户。
	t.Helper()
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	result, err := db.Exec(`
		INSERT INTO aowugong_fastapi_users (username, email, password, is_active)
		VALUES (?, ?, ?, 1)
	`, username, email, passwordHash)
	if err != nil {
		t.Fatalf("insert user error = %v", err)
	}

	// 2. 使用角色编码写入用户角色关联。
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO aowugong_user_roles (user_id, role_id)
		SELECT ?, id FROM aowugong_roles WHERE code = ?
	`, userID, roleCode); err != nil {
		t.Fatalf("assign role error = %v", err)
	}
}

// loginHTTPTestUser 调用登录接口并返回 Bearer 令牌。
func loginHTTPTestUser(t *testing.T, handler http.Handler, username, password string) string {
	// 1. 按 OAuth2PasswordRequestForm 契约发送 URL 编码表单。
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	// 2. 解码并返回令牌字段。
	var response auth.TokenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.AccessToken == "" || response.TokenType != "bearer" {
		t.Fatalf("login response = %#v, want bearer token", response)
	}
	return response.AccessToken
}
