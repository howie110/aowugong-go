package vpn

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSourceCatalogDiscoversAndConvertsClashProfile 验证私有 Clash 文件可归组并转换为通用订阅。
// 输入：包含 VMess 节点的临时 Clash YAML。
// 输出：资源固定提供四种客户端格式，分享链接可解码。
// 副作用：创建并读取测试临时文件。
func TestSourceCatalogDiscoversAndConvertsClashProfile(t *testing.T) {
	// 1. 写入不含真实节点的最小 Clash 配置。
	directory := t.TempDir()
	content := `proxies:
  - name: Test Node
    type: vmess
    server: test.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
    tls: true
`
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 断言资源归组和格式列表稳定。
	catalog := NewSourceCatalog(directory)
	profiles, err := catalog.Profiles()
	if err != nil {
		t.Fatalf("Profiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Code != "demo" || len(profiles[0].Formats) != 4 {
		t.Fatalf("Profiles() = %#v", profiles)
	}

	// 3. 构建配置并确认 v2ray 订阅含标准 VMess 链接。
	configs, err := catalog.Build("demo")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(configs["v2ray"].Body)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if !strings.HasPrefix(string(decoded), "vmess://") || configs["clash"].Body != content {
		t.Errorf("generated configs are invalid")
	}
	if len(configs) != 4 || configs["shadowrocket"].Body == "" || configs["surge"].Body == "" {
		t.Errorf("Build() formats = %#v", configs)
	}
}

// TestSourceCatalogBuildsAvailableLocalPrivateProfiles 验证当前机器私有资源均可转换。
// 输入：开发机 storage/private/vpn；CI 没有私有文件时跳过。
// 输出：每个检测到的资源至少生成一种非空配置。
// 副作用：只读取被 Git 忽略的本地私有文件。
func TestSourceCatalogBuildsAvailableLocalPrivateProfiles(t *testing.T) {
	// 1. 定位仓库私有目录，没有实际资源时跳过。
	directory := filepath.Join("..", "..", "storage", "private", "vpn")
	catalog := NewSourceCatalog(directory)
	profiles, err := catalog.Profiles()
	if err != nil {
		t.Fatalf("Profiles() error = %v", err)
	}
	if len(profiles) == 0 {
		t.Skip("当前环境没有私有 VPN 文件")
	}

	// 2. 逐资源构建并只检查格式和正文非空，不输出任何节点内容。
	for _, profile := range profiles {
		configs, buildErr := catalog.Build(profile.Code)
		if buildErr != nil {
			t.Fatalf("Build(%q) error = %v", profile.Code, buildErr)
		}
		if len(configs) != 4 {
			t.Errorf("Build(%q) returned %d configs, want 4", profile.Code, len(configs))
		}
		for format, config := range configs {
			if strings.TrimSpace(config.Body) == "" {
				t.Errorf("Build(%q) format %q is empty", profile.Code, format)
			}
		}
	}
}
