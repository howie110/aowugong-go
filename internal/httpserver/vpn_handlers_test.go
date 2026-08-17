package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"github.com/howiedata/aowugong-go/internal/vpn"
)

// TestVPNRoutesRequireAdministratorPermission 验证 VPN 资源和设备接口只对管理员开放。
// 输入：隔离 SQLite 中的管理员、投资者和空私有目录。
// 输出：管理员获得摘要，投资者获得 403。
// 副作用：创建并写入隔离 SQLite，执行 httptest 请求。
func TestVPNRoutesRequireAdministratorPermission(t *testing.T) {
	// 1. 组装完整认证、权限和 VPN 路由。
	ctx := context.Background()
	db := testdatabase.Open(t)
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("vpn-http-secret", 72*time.Hour))
	vpnService := vpn.NewService(
		vpn.NewRepository(db), vpn.NewSourceCatalog(t.TempDir()),
		vpn.NewDirectDistributor(""), "vpn-token-secret",
	)
	handler := NewRouter(Dependencies{Auth: authService, RBAC: rbacService, VPN: vpnService})

	// 2. 创建两个角色用户并取得认证令牌。
	createHTTPTestUser(t, db, "vpn-admin", "vpn-admin@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "vpn-investor", "vpn-investor@example.com", "password", rbac.InvestorRoleCode)
	adminToken := loginHTTPTestUser(t, handler, "vpn-admin", "password")
	investorToken := loginHTTPTestUser(t, handler, "vpn-investor", "password")

	// 3. 管理员可读取未配置摘要，投资者不能进入 VPN 页面接口。
	for _, testCase := range []struct {
		token string
		want  int
	}{
		{token: adminToken, want: http.StatusOK},
		{token: investorToken, want: http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/vpn/summary", nil)
		request.Header.Set("Authorization", "Bearer "+testCase.token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != testCase.want {
			t.Errorf("summary status = %d, want %d, body = %s", recorder.Code, testCase.want, recorder.Body.String())
		}
	}
}

// TestVPNSubscriptionUsesDeviceTokenWithoutLogin 验证设备可在未登录工作台时直接拉取订阅。
// 输入：隔离 SQLite、临时 VMess 配置和 Go 直连公开地址。
// 输出：正确 Token 返回正文，错误 Token 返回 404。
// 副作用：创建临时文件、写入 SQLite 并执行 httptest 请求。
func TestVPNSubscriptionUsesDeviceTokenWithoutLogin(t *testing.T) {
	// 1. 准备可生成四种格式的资源并创建有效设备。
	ctx := context.Background()
	db := testdatabase.Open(t)
	directory := t.TempDir()
	content := "proxies:\n  - name: Test\n    type: vmess\n    server: test.example.com\n    port: 443\n    uuid: 00000000-0000-0000-0000-000000000001\n    alterId: 0\n    cipher: auto\n"
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	vpnService := vpn.NewService(
		vpn.NewRepository(db), vpn.NewSourceCatalog(directory),
		vpn.NewDirectDistributor("http://vpn.example.test"), "vpn-token-secret",
	)
	device, err := vpnService.Create(ctx, vpn.CreateRequest{Name: "Android", ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	parsed, err := url.Parse(device.Subscriptions["v2ray"])
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	// 2. 不携带登录令牌请求公开路径，正确密钥返回订阅正文。
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("vpn-http-secret", 72*time.Hour))
	handler := NewRouter(Dependencies{Auth: authService, RBAC: rbacService, VPN: vpnService})
	request := httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) == "" {
		t.Fatalf("subscription status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	// 3. 修改 Token 后统一返回 404，不泄露设备或格式状态。
	parts := strings.Split(parsed.Path, "/")
	parts[len(parts)-2] = strings.Repeat("x", len(parts[len(parts)-2]))
	badRequest := httptest.NewRequest(http.MethodGet, strings.Join(parts, "/"), nil)
	badRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badRecorder, badRequest)
	if badRecorder.Code != http.StatusNotFound {
		t.Errorf("invalid token status = %d, want 404", badRecorder.Code)
	}
}
