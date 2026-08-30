package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestSub2APIClientUsesResponsesContract 验证客户端使用 Bearer 和 OpenAI Responses 请求结构。
// 输入：返回标准 output_text 的本地 HTTP 服务。
// 输出：得到模型文本并核对路径、鉴权和请求字段。
// 副作用：启动进程内测试 HTTP 服务。
func TestSub2APIClientUsesResponsesContract(t *testing.T) {
	// 1. 启动服务并记录完整请求约定。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var payload responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Model != "gpt-5.6-luna" || payload.Input != "测试提示词" || payload.MaxOutputTokens != 321 || payload.Store {
			t.Errorf("payload = %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"测试结果"}]}]}`))
	}))
	defer server.Close()

	// 2. 调用客户端并核对规范化文本。
	client := NewSub2APIClient(config.Sub2API{BaseURL: server.URL + "/v1", APIKey: "test-key"}, "gpt-5.6-luna", server.Client())
	content, err := client.SimpleChat(context.Background(), "测试提示词", 321)
	if err != nil || content != "测试结果" {
		t.Fatalf("SimpleChat() = %q, %v", content, err)
	}
}

// TestSub2APIClientMarksTemporaryUpstreamErrorsRetryable 验证临时上游错误可被业务层识别并回退。
func TestSub2APIClientMarksTemporaryUpstreamErrorsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"Service temporarily unavailable"}}`))
	}))
	defer server.Close()
	client := NewSub2APIClient(config.Sub2API{BaseURL: server.URL, APIKey: "test-key"}, "gpt-5.6-luna", server.Client())

	_, err := client.SimpleChat(context.Background(), "测试", 10)
	var retryable interface{ Retryable() bool }
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("SimpleChat() error = %T %v, want retryable", err, err)
	}
}

// TestSub2APIClientReturnsResponsesError 验证上游错误不会退化为不可读 JSON。
// 输入：返回标准 Responses error.message 的 429 服务。
// 输出：错误包含上游限流说明。
// 副作用：启动进程内测试 HTTP 服务。
func TestSub2APIClientReturnsResponsesError(t *testing.T) {
	// 1. 返回固定限流错误并调用客户端。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"请求过于频繁"}}`))
	}))
	defer server.Close()
	client := NewSub2APIClient(config.Sub2API{BaseURL: server.URL, APIKey: "test-key"}, "gpt-5.6-luna", server.Client())

	// 2. 错误应同时保留 HTTP 状态和上游说明。
	_, err := client.SimpleChat(context.Background(), "测试", 10)
	if err == nil || err.Error() != "Sub2API Responses 返回 HTTP 429: 请求过于频繁" {
		t.Fatalf("SimpleChat() error = %v", err)
	}
}
