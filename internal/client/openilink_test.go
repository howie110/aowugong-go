package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestOpenILinkClientSendsTextWithDefaultRecipient 验证文本消息协议、鉴权和默认接收人。
// 输入：校验请求的本地 OpeniLink Hub 模拟服务。
// 输出：返回 ok=true 的结构化响应。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestOpenILinkClientSendsTextWithDefaultRecipient(t *testing.T) {
	// 1. 创建校验路径、Bearer Token 和 JSON 消息的测试服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bot/v1/message/send" || request.Header.Get("Authorization") != "Bearer app-token" {
			t.Errorf("request = path:%s auth:%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["type"] != "text" || payload["content"] != "测试通知" || payload["to"] != "wx-user" {
			t.Errorf("payload = %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message_id":"m-1"}`))
	}))
	defer server.Close()
	client := NewOpenILinkClient(config.OpenILink{
		HubURL: server.URL, AppToken: "app-token", DefaultTo: "wx-user",
	}, server.Client())

	// 2. 发送文本并核对 Hub 响应。
	result, err := client.SendText(context.Background(), "测试通知", "")
	if err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if result["ok"] != true || result["message_id"] != "m-1" {
		t.Errorf("result = %#v", result)
	}
}

// TestOpenILinkClientRejectsBusinessFailure 验证 Hub 的 ok=false 响应被转换为错误。
// 输入：HTTP 成功但业务失败的 Hub 响应。
// 输出：返回业务错误而不是成功结果。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestOpenILinkClientRejectsBusinessFailure(t *testing.T) {
	// 1. 创建返回业务失败的测试服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"recipient expired"}`))
	}))
	defer server.Close()
	client := NewOpenILinkClient(config.OpenILink{HubURL: server.URL, AppToken: "token"}, server.Client())

	// 2. 发送并确认失败不会被吞掉。
	_, err := client.SendText(context.Background(), "测试", "")
	if err == nil {
		t.Fatal("SendText() error = nil, want business failure")
	}
}
