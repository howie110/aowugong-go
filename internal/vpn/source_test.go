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
	if len(configs) < 4 || configs["shadowrocket"].Body == "" || configs["surge"].Body == "" {
		t.Errorf("Build() formats = %#v", configs)
	}
	if configs["routing"].Body == "" {
		t.Errorf("Build() routing config is empty")
	}
}

// TestSourceCatalogUsesFriendlyMagicRingName 验证魔戒私有资源使用用户熟悉的显示名。
// 输入：名为 v2rayn_mojie.json 的私有资源文件。
// 输出：资源名称显示为“魔戒”，并提供 v2ray 格式。
// 副作用：创建并读取测试临时文件。
func TestSourceCatalogUsesFriendlyMagicRingName(t *testing.T) {
	// 1. 只写入用于发现资源的最小私有文件，不解析节点正文。
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "v2rayn_mojie.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 断言页面资源名称和客户端格式符合魔戒资源约定。
	profiles, err := NewSourceCatalog(directory).Profiles()
	if err != nil {
		t.Fatalf("Profiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Code != "mojie" || profiles[0].Name != "魔戒" {
		t.Fatalf("Profiles() = %#v", profiles)
	}
	hasV2rayFormat := false
	for _, format := range profiles[0].Formats {
		hasV2rayFormat = hasV2rayFormat || format.Code == "v2ray"
	}
	if len(profiles[0].Formats) != 4 || !hasV2rayFormat {
		t.Fatalf("magic ring formats = %#v", profiles[0].Formats)
	}
}

// TestSourceCatalogPrefersNativeXrayForV2ray 验证 v2rayNG 订阅优先使用原生 Xray 配置。
// 输入：同名 Clash 和 Xray 测试配置，Clash 故意开启跳过证书校验。
// 输出：生成节点来自 Xray，且不包含新版 Xray 禁用的不安全参数。
// 副作用：创建并读取测试临时文件。
func TestSourceCatalogPrefersNativeXrayForV2ray(t *testing.T) {
	// 1. 写入同一资源的 Clash 和原生 Xray 配置。
	directory := t.TempDir()
	clashContent := `proxies:
  - name: Clash Node
    type: trojan
    server: clash.example.com
    port: 443
    password: test-password
    sni: clash.example.com
    skip-cert-verify: true
`
	xrayContent := `{
  "outbounds": [{
    "protocol": "trojan",
    "tag": "Native Node",
    "settings": {"servers": [{"address": "native.example.com", "port": 443, "password": "test-password"}]},
    "streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"serverName": "native.example.com", "allowInsecure": false}}
  }]
}`
	for name, content := range map[string]string{
		"clash_demo.yaml":        clashContent,
		"v2rayn_demo.json":       xrayContent,
		"shadowrocket_demo.conf": "native shadowrocket test config",
		"surge_demo.conf":        "native surge test config",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", name, err)
		}
	}

	// 2. 解码订阅并确认只保留原生 Xray 的安全 TLS 节点。
	configs, err := NewSourceCatalog(directory).Build("demo")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(configs["v2ray"].Body)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	link := string(decoded)
	if !strings.Contains(link, "native.example.com") || strings.Contains(link, "clash.example.com") {
		t.Fatalf("v2ray subscription did not use native Xray config")
	}
	if strings.Contains(link, "allowInsecure") || strings.Contains(link, "insecure=1") {
		t.Fatalf("v2ray subscription contains insecure TLS parameters")
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
		if len(configs) < 4 {
			t.Errorf("Build(%q) returned %d configs, want 4", profile.Code, len(configs))
		}
		for format, config := range configs {
			if strings.TrimSpace(config.Body) == "" {
				t.Errorf("Build(%q) format %q is empty", profile.Code, format)
			}
		}
	}
}

func TestSourceCatalogMergesCommonRoutingIntoClientConfigs(t *testing.T) {
	directory := t.TempDir()
	clashContent := `proxies:
  - name: Test
    type: vmess
    server: test.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
proxy-groups:
  - name: PROXY
    type: select
    proxies: [Test, DIRECT]
rules:
  - FINAL,DIRECT
`
	surgeContent := `[General]
loglevel = notify

[Proxy]
Test = vmess, test.example.com, 443, username=00000000-0000-0000-0000-000000000001

[Proxy Group]
PROXY = select, Test, DIRECT

[Rule]
FINAL,DIRECT
`
	shadowrocketContent := `[General]
loglevel = notify

[Proxy]
Test = vmess, test.example.com, 443, username=00000000-0000-0000-0000-000000000001

[Proxy Group]
PROXY = select, Test, DIRECT

[Rule]
FINAL,DIRECT
`
	commonRouting := `{
  "domainStrategy": "IPIfNonMatch",
  "rules": [
    {"type":"field","domain":["domain:example.com"],"outboundTag":"direct"},
    {"type":"field","ip":["1.1.1.1/32"],"outboundTag":"block"}
  ]
}`
	for name, content := range map[string]string{
		"clash_demo.yaml":        clashContent,
		"surge_demo.conf":        surgeContent,
		"shadowrocket_demo.conf": shadowrocketContent,
		"common-routing.json":    commonRouting,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", name, err)
		}
	}

	configs, err := NewSourceCatalog(directory).Build("demo")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, format := range []string{"clash", "surge", "shadowrocket"} {
		if !strings.Contains(configs[format].Body, "DOMAIN-SUFFIX,example.com,DIRECT") || !strings.Contains(configs[format].Body, "IP-CIDR,1.1.1.1/32,REJECT") {
			t.Errorf("%s config does not contain common rules: %q", format, configs[format].Body)
		}
	}
	if !strings.Contains(configs["routing"].Body, `"domainStrategy"`) {
		t.Errorf("routing config = %q", configs["routing"].Body)
	}
}

func TestSourceCatalogBuildsEmptyRoutingWhenCommonFileIsMissing(t *testing.T) {
	directory := t.TempDir()
	content := `proxies:
  - name: Test
    type: vmess
    server: test.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
`
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	configs, err := NewSourceCatalog(directory).Build("demo")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(configs) < 4 || !strings.Contains(configs["routing"].Body, `"rules": []`) {
		t.Fatalf("configs = %#v", configs)
	}
}

func TestSourceCatalogRejectsInvalidCommonRouting(t *testing.T) {
	directory := t.TempDir()
	content := `proxies:
  - name: Test
    type: vmess
    server: test.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
`
	for name, value := range map[string]string{
		"clash_demo.yaml":     content,
		"common-routing.json": "{",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(value), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", name, err)
		}
	}
	if _, err := NewSourceCatalog(directory).Build("demo"); err == nil {
		t.Fatal("Build() error = nil, want invalid common routing error")
	}
}
