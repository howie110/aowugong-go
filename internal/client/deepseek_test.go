package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestDeepSeekClientReturnsFirstAssistantContent 验证兼容 OpenAI 的聊天补全请求。
// 输入：校验 Bearer 鉴权并返回单个 choice 的本地 HTTP 服务。
// 输出：返回第一条 assistant content。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestDeepSeekClientReturnsFirstAssistantContent(t *testing.T) {
	// 1. 创建校验路径和鉴权头的测试服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"ok\"}"}}]}`))
	}))
	defer server.Close()
	client := NewDeepSeekClient(config.DeepSeek{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}, server.Client())

	// 2. 调用单轮聊天并核对文本。
	content, err := client.SimpleChat(context.Background(), "测试提示词", 1600)
	if err != nil {
		t.Fatalf("SimpleChat() error = %v", err)
	}
	if content != `{"summary":"ok"}` {
		t.Errorf("content = %q", content)
	}
}
