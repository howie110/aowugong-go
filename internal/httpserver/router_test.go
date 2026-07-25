package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewRouterServesHealth 验证健康检查端点返回 JSON 状态。
// 输入：使用临时静态目录的测试路由。
// 输出：健康接口返回 200 和 ok 状态。
// 副作用：创建临时目录并执行 HTTP 请求。
func TestNewRouterServesHealth(t *testing.T) {
	// 1. 请求健康检查端点。
	recorder := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	// 2. 断言状态码、内容类型和响应内容。
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response["status"] != "ok" {
		t.Errorf("status response = %q, want ok", response["status"])
	}
}

// TestNewRouterReturnsJSONForUnknownAPI 验证未知 API 使用 JSON 错误信封。
// 输入：不存在的 API 路径。
// 输出：返回 404 和 not_found 错误码。
// 副作用：创建临时目录并执行 HTTP 请求。
func TestNewRouterReturnsJSONForUnknownAPI(t *testing.T) {
	// 1. 请求未注册的 API 路径。
	recorder := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))

	// 2. 断言 404 和结构化错误响应。
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	assertErrorEnvelope(t, recorder, "not_found")
}

// TestNewRouterFallsBackToSPAIndex 验证非资源路径会回退到 SPA 入口文件。
// 输入：前端页面路由路径。
// 输出：返回临时 index.html 内容。
// 副作用：创建临时静态文件并执行 HTTP 请求。
func TestNewRouterFallsBackToSPAIndex(t *testing.T) {
	// 1. 创建包含 SPA 入口文件的静态目录。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<main>app</main>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 请求前端路由并断言返回入口内容。
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{StaticDir: dir}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/jobs/42", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "<main>app</main>" {
		t.Errorf("body = %q, want SPA index content", body)
	}
}

// TestNewRouterDoesNotFallbackForMissingAsset 验证缺失静态资源返回 JSON 404。
// 输入：不存在的带扩展名资源路径。
// 输出：返回 404 而不是 SPA 入口。
// 副作用：创建临时目录并执行 HTTP 请求。
func TestNewRouterDoesNotFallbackForMissingAsset(t *testing.T) {
	// 1. 创建仅包含 SPA 入口文件的静态目录。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<main>app</main>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 请求不存在的带扩展名资源。
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{StaticDir: dir}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	assertErrorEnvelope(t, recorder, "not_found")
}

// newTestRouter 创建使用临时静态目录的路由器。
// 输入：t 管理临时目录生命周期。
// 输出：返回包含固定 index.html 的测试路由。
// 副作用：创建临时前端文件。
func newTestRouter(t *testing.T) http.Handler {
	// 1. 返回不依赖静态资源的测试路由器。
	t.Helper()
	return NewRouter(Dependencies{})
}

// assertErrorEnvelope 断言响应符合统一 JSON 错误信封格式。
// 输入：测试句柄、HTTP 记录器和期望错误码。
// 输出：无；格式或错误码不符时报告测试失败。
// 副作用：读取记录器响应体。
func assertErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, wantCode string) {
	// 1. 解码并验证错误信封字段。
	t.Helper()
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q", response.Error.Code, wantCode)
	}
	if response.Error.Message == "" {
		t.Error("error.message is empty")
	}
}
