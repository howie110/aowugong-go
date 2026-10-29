package httpserver

import "testing"

// TestBuildNotificationExternalConfig 验证外部调用配置只读取 external 项目。
// 输入：包含和不包含 external Token 的映射。
// 输出：配置状态、Token 和限制正确。
// 副作用：无。
func TestBuildNotificationExternalConfig(t *testing.T) {
	// 1. 配置 external 时返回固定端点和限制。
	config := buildNotificationExternalConfig(map[string]string{
		"external": "test-token",
		"other":    "other-token",
	})
	if !config.Configured || config.Token != "test-token" {
		t.Fatalf("config = %#v", config)
	}
	if config.Endpoint != "/api/v1/notifications/wechat" || config.Project != "external" ||
		config.MaxMessageBytes != 2048 || config.MaxRequestBytes != 16384 || config.RateLimitPerMinute != 10 {
		t.Fatalf("config limits = %#v", config)
	}

	// 2. 未配置 external 时明确返回未启用，不读取其他项目 Token。
	empty := buildNotificationExternalConfig(map[string]string{"other": "other-token"})
	if empty.Configured || empty.Token != "" {
		t.Fatalf("empty config = %#v", empty)
	}
}
