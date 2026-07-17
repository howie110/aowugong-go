package monitoring

import (
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestBuildTargetsIncludesDefaultsAndDedupesExtras 验证默认目标、业务目标和额外目标按 code 去重。
func TestBuildTargetsIncludesDefaultsAndDedupesExtras(t *testing.T) {
	// 1. 为两个内部服务分别配置本机探测地址和公网展示地址。
	cfg := config.Clients{
		ArticleRSSURL:       "http://127.0.0.1:5000/api/rss/all",
		WeChatRSSMonitorURL: "http://8.138.123.59:5000/api/admin/status",
		OpenILink: config.OpenILink{
			HubURL:     "http://127.0.0.1:9800",
			MonitorURL: "http://8.138.123.59:9800/",
		},
		ServiceMonitorTargets: `[
			{"code":"demo","name":"Demo","url":"https://example.com/health"},
			{"code":"aowugong-blog","name":"duplicate","url":"https://invalid.example"}
		]`,
	}

	// 2. 结果应包含五个唯一目标，页面地址使用公网，探测地址使用本机链路。
	targets := BuildTargets(cfg)
	if len(targets) != 5 {
		t.Fatalf("target count = %d, want 5: %#v", len(targets), targets)
	}
	if targets[2].URL != "http://8.138.123.59:5000/api/admin/status" || targets[2].ProbeURL != "http://127.0.0.1:5000/api/admin/status" {
		t.Errorf("wechat target = %#v", targets[2])
	}
	if targets[3].Code != "openilink-hub" || targets[3].URL != "http://8.138.123.59:9800/" || targets[3].ProbeURL != "http://127.0.0.1:9800/bot/v1/message/send" {
		t.Errorf("openilink target = %#v", targets[3])
	}
}

// TestValidateWeChatRSSLoginPayload 验证登录过期会被转换为可读错误。
func TestValidateWeChatRSSLoginPayload(t *testing.T) {
	// 1. 健康载荷必须通过。
	healthy := map[string]any{"authenticated": true, "loggedIn": true, "isExpired": false}
	if message := ValidateWeChatRSSLoginPayload(healthy); message != "" {
		t.Errorf("healthy message = %q, want empty", message)
	}

	// 2. 过期载荷必须包含登录异常说明。
	expired := map[string]any{"authenticated": false, "loggedIn": false, "isExpired": true, "account": "嗷呜公", "status": "登录已过期"}
	if message := ValidateWeChatRSSLoginPayload(expired); message == "" {
		t.Error("expired message is empty")
	}
}
