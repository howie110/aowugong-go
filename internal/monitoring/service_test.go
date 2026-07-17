package monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
)

// TestCheckTargetUsesProbeURLAndKeepsPublicURL 验证探测走本机地址而结果保留公网跳转地址。
func TestCheckTargetUsesProbeURLAndKeepsPublicURL(t *testing.T) {
	// 1. 准备一个不可用的公网展示端点和一个正常的内部探测端点。
	var publicRequests atomic.Int32
	publicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		publicRequests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer publicServer.Close()
	var probeRequests atomic.Int32
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probeRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer probeServer.Close()

	// 2. 执行普通服务探测并保留对外展示地址。
	service := NewService(nil, client.NewMonitoringClient(probeServer.Client()), config.Clients{})
	result := service.checkTarget(context.Background(), Target{
		Code: "demo", Name: "Demo", URL: publicServer.URL, ProbeURL: probeServer.URL,
	})

	// 3. 断言只访问内部端点，且 API 结果仍返回公网地址。
	if result.Status != "up" || result.TargetURL != publicServer.URL {
		t.Fatalf("result = %#v", result)
	}
	if publicRequests.Load() != 0 || probeRequests.Load() != 1 {
		t.Fatalf("request counts: public=%d probe=%d", publicRequests.Load(), probeRequests.Load())
	}
}

// TestCheckOpenILinkUsesHTTPWithoutReadingDatabase 验证 OpeniLink 监控只调用内部 HTTP 接口。
func TestCheckOpenILinkUsesHTTPWithoutReadingDatabase(t *testing.T) {
	// 1. 准备返回预期空内容错误的 OpeniLink 发送接口。
	var requests atomic.Int32
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer app-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"content is required for text messages"}`))
	}))
	defer probeServer.Close()

	// 2. 配置一个不可作为 SQLite 打开的目录，执行 OpeniLink 监控。
	service := NewService(nil, client.NewMonitoringClient(probeServer.Client()), config.Clients{
		OpenILink: config.OpenILink{
			AppToken: "app-token", DefaultTo: "wx-user", DBPath: t.TempDir(),
		},
	})
	result := service.checkTarget(context.Background(), Target{
		Code: "openilink-hub", Name: "OpeniLink", URL: "http://public.example/", ProbeURL: probeServer.URL,
	})

	// 3. HTTP 空内容探测应被识别为正常，且只发起一次请求。
	if result.Status != "up" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusOK {
		t.Fatalf("result = %#v", result)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}
