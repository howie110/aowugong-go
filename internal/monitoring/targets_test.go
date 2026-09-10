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
			MonitorURL: "https://miniflux.aowugong.top/",
		},
		ServiceMonitorTargets: `[
			{"code":"demo","name":"Demo","url":"https://example.com/health"},
			{"code":"aowugong-blog","name":"duplicate","url":"https://invalid.example"}
		]`,
	}

	// 2. 当前五个服务加一个额外目标，重复 code 只保留第一项。
	targets := BuildTargets(cfg)
	if len(targets) != 6 {
		t.Fatalf("target count = %d, want 6: %#v", len(targets), targets)
	}
	byCode := make(map[string]Target, len(targets))
	for _, target := range targets {
		byCode[target.Code] = target
	}
	if target := byCode["miniflux"]; target.URL != "https://miniflux.aowugong.top/" || target.ProbeURL != "http://127.0.0.1:5000/healthcheck" {
		t.Errorf("miniflux target = %#v", target)
	}
	if target := byCode["aowugong-blog"]; target.URL != "https://blog.aowugong.top/" {
		t.Errorf("blog target was overridden: %#v", target)
	}
}

func TestBuildTargetsUsesCurrentPublicServices(t *testing.T) {
	targets := BuildTargets(config.Clients{})
	want := map[string]string{
		"aowugong-home": "https://aowugong.top/",
		"aowugong-blog": "https://blog.aowugong.top/",
		"nextflux":      "https://nextflux.aowugong.top/",
		"vaultwarden":   "https://vault.aowugong.top/",
	}
	if len(targets) != len(want) {
		t.Fatalf("target count = %d, want %d: %#v", len(targets), len(want), targets)
	}
	for _, target := range targets {
		address, ok := want[target.Code]
		if !ok {
			t.Errorf("unexpected or retired target: %#v", target)
			continue
		}
		if target.URL != address || target.ProbeURL != address {
			t.Errorf("target %q: display=%q probe=%q, want %q", target.Code, target.URL, target.ProbeURL, address)
		}
		delete(want, target.Code)
	}
	for code := range want {
		t.Errorf("missing target %q", code)
	}
}
