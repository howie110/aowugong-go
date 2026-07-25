package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestDevelopmentRouterServesLocalSPAAndProxiesAPI 验证本地页面和线上数据使用不同来源。
// 输入：测试静态目录和记录 API 请求的远端服务。
// 输出：页面来自本地 index，API 原样转发路径、查询和令牌。
// 副作用：启动测试 HTTP 服务并读取临时静态文件。
func TestDevelopmentRouterServesLocalSPAAndProxiesAPI(t *testing.T) {
	// 1. 创建本地 SPA 入口和记录请求的模拟线上 API。
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("local-spa"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		// 2. 把收到的关键请求信息返回给测试断言。
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, request.URL.RequestURI()+"|"+request.Header.Get("Authorization"))
	}))
	defer upstream.Close()
	handler, err := NewDevelopmentRouter(staticDir, upstream.URL)
	if err != nil {
		t.Fatalf("NewDevelopmentRouter() error = %v", err)
	}

	// 3. 普通页面必须使用本地前端产物。
	pageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pageRecorder, httptest.NewRequest(http.MethodGet, "/database", nil))
	if pageRecorder.Code != http.StatusOK || pageRecorder.Body.String() != "local-spa" {
		t.Errorf("page = %d %q", pageRecorder.Code, pageRecorder.Body.String())
	}

	// 4. API 必须完整转发给线上服务。
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/v1/database/summary?fresh=1", nil)
	apiRequest.Header.Set("Authorization", "Bearer test-token")
	apiRecorder := httptest.NewRecorder()
	handler.ServeHTTP(apiRecorder, apiRequest)
	if apiRecorder.Code != http.StatusOK ||
		apiRecorder.Body.String() != "/api/v1/database/summary?fresh=1|Bearer test-token" {
		t.Errorf("api = %d %q", apiRecorder.Code, apiRecorder.Body.String())
	}
}

// TestDevelopmentRouterRejectsUnsafeUpstream 验证开发代理不接受带身份或业务路径的地址。
// 输入：两个不符合根地址约束的 URL。
// 输出：均返回配置错误。
// 副作用：无。
func TestDevelopmentRouterRejectsUnsafeUpstream(t *testing.T) {
	// 1. 逐个验证身份信息和业务路径都被拒绝。
	for _, upstream := range []string{"http://user:pass@example.com", "http://example.com/api"} {
		if _, err := NewDevelopmentRouter("", upstream); err == nil {
			t.Errorf("NewDevelopmentRouter(%q) error = nil", upstream)
		}
	}
}
