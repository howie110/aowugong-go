package monitoring

import (
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestBuildTargetsIncludesDefaultsAndDedupesExtras 验证默认目标、业务目标和额外目标按 code 去重。
func TestBuildTargetsIncludesDefaultsAndDedupesExtras(t *testing.T) {
	// 1. 配置 WeChatRSS、OpeniLink 和一个与博客 code 冲突的额外目标。
	cfg := config.Clients{
		WeChatRSSMonitorURL: "http://127.0.0.1:5000/api/admin/status",
		OpenILink:           config.OpenILink{MonitorURL: "http://127.0.0.1:9800/"},
		ServiceMonitorTargets: `[
			{"code":"demo","name":"Demo","url":"https://example.com/health"},
			{"code":"aowugong-blog","name":"duplicate","url":"https://invalid.example"}
		]`,
	}

	// 2. 结果应包含五个唯一目标且 OpeniLink 指向发送接口。
	targets := BuildTargets(cfg)
	if len(targets) != 5 {
		t.Fatalf("target count = %d, want 5: %#v", len(targets), targets)
	}
	if targets[3].Code != "openilink-hub" || targets[3].URL != "http://127.0.0.1:9800/bot/v1/message/send" {
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
