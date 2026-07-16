package weread

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
)

// TestDashboardKeepsSummaryMetricsInsideWeRead 验证兼容聚合接口完整合并三个子接口。
// 输入：返回最小有效数据的微信读书模拟网关。
// 输出：顶层和 weread 内部都包含相同 metrics，且保留 summary、progress 和 heatmap。
// 副作用：启动本地 httptest 服务并发出模拟 HTTP 请求。
func TestDashboardKeepsSummaryMetricsInsideWeRead(t *testing.T) {
	// 1. 模拟 Dashboard 全部调用会用到的微信读书接口。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var response map[string]any
		switch payload["api_name"] {
		case "/readdata/detail":
			response = map[string]any{"totalReadTime": 0, "readDays": 0, "readTimes": map[string]any{}}
		case "/user/notebooks":
			response = map[string]any{"books": []any{}, "totalBookCount": 0, "totalNoteCount": 0, "hasMore": 0}
		case "/shelf/sync":
			response = map[string]any{"books": []any{}}
		default:
			t.Errorf("unexpected api_name = %v", payload["api_name"])
			response = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()
	service := NewService(client.NewWeReadClient(config.WeRead{
		GatewayURL: server.URL, APIKey: "test-key", SkillVersion: "1",
	}, server.Client()))

	// 2. 调用正式聚合入口并读取嵌套兼容对象。
	result, err := service.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	nested, ok := result["weread"].(map[string]any)
	if !ok {
		t.Fatalf("weread = %#v, want object", result["weread"])
	}

	// 3. 核对旧接口的完整展开字段仍然存在。
	if !reflect.DeepEqual(nested["metrics"], result["metrics"]) {
		t.Fatalf("weread.metrics = %#v, top metrics = %#v", nested["metrics"], result["metrics"])
	}
	for _, key := range []string{"summary", "recent_books", "progress_books", "heatmap"} {
		if _, exists := nested[key]; !exists {
			t.Errorf("weread.%s is missing", key)
		}
	}
}
