package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestWeReadClientRetriesLatestSkillVersion 验证网关提示新版本时只自动重试一次。
// 输入：先返回版本提示、再返回成功结果的本地测试网关。
// 输出：客户端使用最新版本完成第二次请求。
// 副作用：启动本地 HTTP 测试服务。
func TestWeReadClientRetriesLatestSkillVersion(t *testing.T) {
	// 1. 创建先返回升级信息、再返回业务数据的测试网关。
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		callCount++
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"upgrade_info": map[string]any{"latest_version": "2.0.0"}})
			return
		}
		if payload["skill_version"] != "2.0.0" {
			t.Errorf("skill_version = %v, want 2.0.0", payload["skill_version"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"totalReadTime": 60})
	}))
	defer server.Close()

	// 2. 客户端应返回第二次响应并保留调用次数为二。
	client := NewWeReadClient(config.WeRead{GatewayURL: server.URL, APIKey: "token", SkillVersion: "1.0.0"}, &http.Client{Timeout: time.Second})
	response, err := client.Call(context.Background(), "/readdata/detail", map[string]any{"mode": "overall"})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if callCount != 2 || IntValue(response["totalReadTime"]) != 60 {
		t.Errorf("callCount/response = %d/%v", callCount, response)
	}
}

// TestWeReadClientRequiresAPIKey 验证未配置密钥时不会请求外部网关。
// 输入：缺少 API Key 的微信读书配置。
// 输出：请求在联网前返回配置错误。
// 副作用：无。
func TestWeReadClientRequiresAPIKey(t *testing.T) {
	// 1. 创建空密钥客户端并尝试调用。
	client := NewWeReadClient(config.WeRead{GatewayURL: "http://127.0.0.1", SkillVersion: "1.0.0"}, &http.Client{Timeout: time.Second})
	_, err := client.Call(context.Background(), "/shelf/sync", nil)

	// 2. 调用必须返回明确配置错误。
	if err == nil {
		t.Fatal("Call() error = nil, want missing key error")
	}
}
