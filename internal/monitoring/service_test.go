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
// 输入：公网展示地址和本机测试探测地址。
// 输出：探测成功且结果保留公网地址。
// 副作用：启动本地 HTTP 服务并写入临时 SQLite。
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

// TestCheckTargetRetriesTransientFailure 验证普通服务首次失败后会复检一次。
// 输入：第一次返回 503、第二次返回 200 的本地 HTTP 服务。
// 输出：最终监控状态为正常，并且服务共收到两次请求。
// 副作用：启动本地 HTTP 服务。
func TestCheckTargetRetriesTransientFailure(t *testing.T) {
	// 1. 准备首次失败、复检成功的普通 HTTP 服务。
	var requests atomic.Int32
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer probeServer.Close()

	// 2. 执行探测并要求复检结果覆盖首次瞬时失败。
	service := NewService(nil, client.NewMonitoringClient(probeServer.Client()), config.Clients{})
	result := service.checkTarget(context.Background(), Target{
		Code: "demo", Name: "Demo", URL: probeServer.URL,
	})

	// 3. 连续请求次数和最终健康状态必须与复检规则一致。
	if result.Status != "up" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusOK {
		t.Fatalf("result = %#v", result)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

// TestCheckTargetKeepsSecondFailure 验证普通服务连续失败时仍会告警。
// 输入：始终返回 503 的本地 HTTP 服务。
// 输出：最终监控状态为异常，并保留第二次 HTTP 状态。
// 副作用：启动本地 HTTP 服务。
func TestCheckTargetKeepsSecondFailure(t *testing.T) {
	// 1. 准备连续返回服务不可用的普通 HTTP 服务。
	var requests atomic.Int32
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer probeServer.Close()

	// 2. 执行探测并要求连续失败保持异常状态。
	service := NewService(nil, client.NewMonitoringClient(probeServer.Client()), config.Clients{})
	result := service.checkTarget(context.Background(), Target{
		Code: "demo", Name: "Demo", URL: probeServer.URL,
	})

	// 3. 两次探测均失败后必须保留最终错误，不得误报正常。
	if result.Status != "down" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("result = %#v", result)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

// TestCheckOpenILinkUsesHTTPWithoutReadingDatabase 验证 OpeniLink 监控只调用内部 HTTP 接口。
// 输入：返回健康状态的本地 OpeniLink 模拟服务。
// 输出：监控结果正常且未依赖 OpeniLink 数据库文件。
// 副作用：启动本地 HTTP 服务并写入临时 SQLite。
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
