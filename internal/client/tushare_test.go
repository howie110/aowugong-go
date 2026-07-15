package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestTushareClientDailyUsesNativeHTTPProtocol 验证日线接口请求和表格响应转换。
// 输入：校验 api_name、token 和日期参数的本地 Tushare 模拟服务。
// 输出：返回字段映射正确且日期规范化的日线记录。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestTushareClientDailyUsesNativeHTTPProtocol(t *testing.T) {
	// 1. 创建模拟 Tushare 服务并校验原生 POST 协议。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Method != http.MethodPost || payload["api_name"] != "daily" || payload["token"] != "test-token" {
			t.Errorf("request = method:%s payload:%#v", request.Method, payload)
		}
		params, _ := payload["params"].(map[string]any)
		if params["start_date"] != "20260102" || params["end_date"] != "20260102" {
			t.Errorf("params = %#v", params)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":null,"data":{"fields":["ts_code","trade_date","open","high","low","close","pre_close","change","pct_chg","vol","amount"],"items":[["000001.SZ","20260102",10,12,9,11,9.5,1.5,15.8,1000,11000]]}}`))
	}))
	defer server.Close()
	client := NewTushareClient(config.Tushare{BaseURL: server.URL, Token: "test-token"}, server.Client())

	// 2. 调用日线接口并核对结构化转换结果。
	rows, err := client.Daily(context.Background(), "2026-01-02", "2026-01-02")
	if err != nil {
		t.Fatalf("Daily() error = %v", err)
	}
	if len(rows) != 1 || rows[0].TSCode != "000001.SZ" || rows[0].TradeDate != "2026-01-02" || rows[0].Close != 11 {
		t.Fatalf("rows = %#v", rows)
	}
}

// TestTushareClientReturnsBusinessError 验证上游业务错误不会被当作空数据。
// 输入：HTTP 200 但 code 非零的 Tushare 响应。
// 输出：返回包含上游消息的错误。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestTushareClientReturnsBusinessError(t *testing.T) {
	// 1. 创建返回 Tushare 业务错误的测试服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":40203,"msg":"抱歉，您没有访问该接口的权限","data":null}`))
	}))
	defer server.Close()
	client := NewTushareClient(config.Tushare{BaseURL: server.URL, Token: "test-token"}, server.Client())

	// 2. 调用并确认业务错误被返回。
	_, err := client.Daily(context.Background(), "2026-01-02", "2026-01-02")
	if err == nil {
		t.Fatal("Daily() error = nil, want business error")
	}
}
