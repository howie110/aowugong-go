package monitoring

import (
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestBuildTargetsIncludesDefaultsAndDedupesExtras 验证默认目标、业务目标和额外目标按 code 去重。
// 输入：默认客户端地址和包含重复 code 的额外监控配置。
// 输出：返回去重且探测地址正确的目标列表。
// 副作用：无。
func TestBuildTargetsIncludesDefaultsAndDedupesExtras(t *testing.T) {
	// 1. 配置 Miniflux 本机探测地址、公网展示地址和额外目标。
	cfg := config.Clients{
		Miniflux: config.Miniflux{
			BaseURL:    "http://127.0.0.1:5000",
			MonitorURL: "http://8.138.123.59:5000/",
		},
		ServiceMonitorTargets: `[
			{"code":"demo","name":"Demo","url":"https://example.com/health"},
			{"code":"aowugong-blog","name":"duplicate","url":"https://invalid.example"}
		]`,
	}

	// 2. 结果应包含四个唯一目标，重复 code 只保留第一项。
	targets := BuildTargets(cfg)
	if len(targets) != 4 {
		t.Fatalf("target count = %d, want 4: %#v", len(targets), targets)
	}
	if targets[2].Code != "miniflux" || targets[2].URL != "http://8.138.123.59:5000/" || targets[2].ProbeURL != "http://127.0.0.1:5000/healthcheck" {
		t.Errorf("miniflux target = %#v", targets[2])
	}
}
