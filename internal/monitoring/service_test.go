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
