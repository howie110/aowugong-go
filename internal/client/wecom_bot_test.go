package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestWeComBotClientSendsText(t *testing.T) {
	var payload map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "qyapi.weixin.qq.com" || request.URL.Query().Get("key") != "test-key" {
			t.Fatalf("request URL = %s", request.URL.Redacted())
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	client := NewWeComBotClient(config.WeComBot{
		WebhookURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key",
	}, httpClient)
	if err := client.SendText(context.Background(), "测试通知"); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if payload["msgtype"] != "text" {
		t.Fatalf("payload = %#v", payload)
	}
	textPayload, ok := payload["text"].(map[string]any)
	if !ok || textPayload["content"] != "测试通知" {
		t.Fatalf("text payload = %#v", payload["text"])
	}
}

func TestWeComBotClientRejectsUnofficialWebhook(t *testing.T) {
	client := NewWeComBotClient(config.WeComBot{WebhookURL: "https://example.com/cgi-bin/webhook/send?key=secret"}, nil)
	if client.Configured() {
		t.Fatal("Configured() = true, want false")
	}
	if err := client.SendText(context.Background(), "测试"); err == nil {
		t.Fatal("SendText() error = nil, want validation error")
	}
}

func TestWeComBotClientRejectsBusinessFailure(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":93000,"errmsg":"invalid webhook"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	client := NewWeComBotClient(config.WeComBot{
		WebhookURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key",
	}, httpClient)
	if err := client.SendText(context.Background(), "测试"); err == nil {
		t.Fatal("SendText() error = nil, want business error")
	}
}

func TestTruncateUTF8BytesPreservesValidText(t *testing.T) {
	result := truncateUTF8Bytes(strings.Repeat("文", 900), weComTextLimit)
	if len(result) > weComTextLimit || !strings.HasSuffix(result, "[内容已截断]") {
		t.Fatalf("truncated length = %d suffix = %q", len(result), result[len(result)-20:])
	}
}
