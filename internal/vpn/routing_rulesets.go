package vpn

import (
	"fmt"
	"strings"
)

const chinaRulesRoot = "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/"

// chinaDomainRule 将公共国内域名集合映射为各客户端的外部域名集。
// geosite:cn 是公共配置的逻辑标识；此处不要求客户端安装 Xray geosite 数据。
func chinaDomainRule(client, target string) string {
	if client == "clash" {
		return "RULE-SET,aowugong-cn," + target
	}
	kind := "Surge"
	if client == "shadowrocket" {
		kind = "Shadowrocket"
	}
	return "DOMAIN-SET," + chinaRulesRoot + kind + "/ChinaMax/ChinaMax_Domain.list," + target
}

// addChinaDomainProvider 仅在公共规则引用国内域名集时注入提供者，保留其他源配置。
// 下载与缓存由客户端完成，后端生成订阅时不联网。
func addChinaDomainProvider(document map[string]any, routing commonRouting) error {
	needed := false
	for _, rule := range routing.Rules {
		for _, domain := range rule.Domain {
			needed = needed || strings.EqualFold(strings.TrimSpace(domain), "geosite:cn")
		}
	}
	if !needed {
		return nil
	}
	providers, ok := document["rule-providers"].(map[string]any)
	if document["rule-providers"] != nil && !ok {
		return fmt.Errorf("Clash rule-providers 格式无效")
	}
	if providers == nil {
		providers = make(map[string]any)
	}
	if _, exists := providers["aowugong-cn"]; exists {
		return fmt.Errorf("Clash 源配置占用了公共规则集名称 aowugong-cn")
	}
	providers["aowugong-cn"] = map[string]any{
		"type": "http", "behavior": "domain", "interval": 86400,
		"url":  chinaRulesRoot + "Clash/ChinaMax/ChinaMax_Domain.yaml",
		"path": "./rule-providers/aowugong-cn.yaml",
	}
	document["rule-providers"] = providers
	return nil
}
