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

// TestBuildTransition 验证故障、恢复和连续相同状态的判断。
// 输入：上一轮和当前的结果组合。
// 输出：只在 up→down、首次 down、down→up 时产生事件，连续相同状态不产生。
// 副作用：无。
func TestBuildTransition(t *testing.T) {
	tests := []struct {
		name         string
		previous     Result
		current      Result
		wantChanged  bool
		wantPrevious string
	}{
		{name: "首次异常", previous: Result{}, current: Result{Status: "down"}, wantChanged: true},
		{name: "正常转异常", previous: Result{Status: "up"}, current: Result{Status: "down"}, wantChanged: true, wantPrevious: "up"},
		{name: "持续异常", previous: Result{Status: "down"}, current: Result{Status: "down"}, wantChanged: false},
		{name: "异常恢复", previous: Result{Status: "down"}, current: Result{Status: "up"}, wantChanged: true, wantPrevious: "down"},
		{name: "持续正常", previous: Result{Status: "up"}, current: Result{Status: "up"}, wantChanged: false},
		{name: "首次正常", previous: Result{}, current: Result{Status: "up"}, wantChanged: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, changed := buildTransition(test.previous, test.current)
			if changed != test.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, test.wantChanged)
			}
			if changed && transition.PreviousStatus != test.wantPrevious {
				t.Fatalf("previous status = %q, want %q", transition.PreviousStatus, test.wantPrevious)
			}
		})
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
