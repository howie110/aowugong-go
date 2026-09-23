package vpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type vmessShare struct {
	Name    string `json:"ps"`
	Server  string `json:"add"`
	Port    string `json:"port"`
	UUID    string `json:"id"`
	AlterID string `json:"aid"`
	Cipher  string `json:"scy"`
	Network string `json:"net"`
	Host    string `json:"host"`
	Path    string `json:"path"`
	TLS     string `json:"tls"`
	SNI     string `json:"sni"`
}

// decodeVMessSubscription 解析本项目生成的 Base64 VMess 分享链接订阅。
// 输入：subscription 是 v2rayN/v2rayNG 订阅正文。
// 输出：返回全部结构化 VMess 节点；包含其他协议或无节点时返回错误。
// 副作用：无。
func decodeVMessSubscription(subscription string) ([]vmessShare, error) {
	// 1. 解码订阅外层 Base64 并逐行校验 VMess 链接。
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(subscription))
	if err != nil {
		return nil, fmt.Errorf("解码标准订阅: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	nodes := make([]vmessShare, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "vmess://") {
			return nil, fmt.Errorf("自动补齐格式暂不支持节点协议 %q", strings.SplitN(line, ":", 2)[0])
		}
		payload, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "vmess://"))
		if decodeErr != nil {
			return nil, fmt.Errorf("解码 VMess 节点: %w", decodeErr)
		}
		var node vmessShare
		if decodeErr := json.Unmarshal(payload, &node); decodeErr != nil {
			return nil, fmt.Errorf("解析 VMess 节点: %w", decodeErr)
		}
		if node.Name == "" || node.Server == "" || node.Port == "" || node.UUID == "" {
			return nil, fmt.Errorf("VMess 节点缺少名称、地址、端口或 UUID")
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("标准订阅没有 VMess 节点")
	}
	return nodes, nil
}

// vmessSubscriptionToClash 把 VMess 标准订阅转换为 Clash/FlClash YAML。
// 输入：subscription 是标准订阅，profileCode 用于代理组名称。
// 输出：返回可直接导入的完整 Clash YAML。
// 副作用：无。
func vmessSubscriptionToClash(subscription, profileCode string) (string, error) {
	// 1. 将每个 VMess 分享模型映射为 Clash 节点。
	nodes, err := decodeVMessSubscription(subscription)
	if err != nil {
		return "", err
	}
	proxies := make([]map[string]any, 0, len(nodes))
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		port, parseErr := strconv.Atoi(node.Port)
		if parseErr != nil {
			return "", fmt.Errorf("节点 %s 端口无效: %w", node.Name, parseErr)
		}
		proxy := map[string]any{
			"name": node.Name, "type": "vmess", "server": node.Server, "port": port,
			"uuid": node.UUID, "alterId": parseIntDefault(node.AlterID, 0),
			"cipher": firstNonEmpty(node.Cipher, "auto"), "network": firstNonEmpty(node.Network, "tcp"),
		}
		if node.TLS != "" {
			proxy["tls"] = true
			proxy["servername"] = firstNonEmpty(node.SNI, node.Host)
		}
		if node.Network == "ws" {
			proxy["ws-opts"] = map[string]any{"path": node.Path, "headers": map[string]any{"Host": node.Host}}
		}
		proxies = append(proxies, proxy)
		names = append(names, node.Name)
	}

	// 2. 生成包含选择组和兜底规则的最小完整配置。
	document := map[string]any{
		"mixed-port": 7890,
		"mode":       "rule",
		"proxies":    proxies,
		"proxy-groups": []map[string]any{{
			"name": profileDisplayName(profileCode), "type": "select", "proxies": append(names, "DIRECT"),
		}},
		"rules": []string{"MATCH," + profileDisplayName(profileCode)},
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("编码 Clash YAML: %w", err)
	}
	return string(encoded), nil
}

// vmessSubscriptionToSurge 把 VMess 标准订阅转换为最小 Surge 配置。
// 输入：subscription 是标准订阅正文。
// 输出：返回包含节点、选择组和最终规则的 Surge 配置。
// 副作用：无。
func vmessSubscriptionToSurge(subscription string) (string, error) {
	// 1. 将 VMess 节点转换为 Surge Proxy 行。
	nodes, err := decodeVMessSubscription(subscription)
	if err != nil {
		return "", err
	}
	lines := []string{"[General]", "loglevel = notify", "", "[Proxy]"}
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		name := strings.NewReplacer(",", "，", "=", "-").Replace(node.Name)
		parts := []string{name + " = vmess", node.Server, node.Port, "username=" + node.UUID}
		if node.TLS != "" {
			parts = append(parts, "tls=true", "sni="+firstNonEmpty(node.SNI, node.Host))
		}
		if node.Network == "ws" {
			parts = append(parts, "ws=true", "ws-path="+node.Path)
			if node.Host != "" {
				parts = append(parts, "ws-headers=Host:"+node.Host)
			}
		}
		lines = append(lines, strings.Join(parts, ", "))
		names = append(names, name)
	}

	// 2. 添加节点选择组和最终代理规则。
	lines = append(lines, "", "[Proxy Group]", "PROXY = select, "+strings.Join(append(names, "DIRECT"), ", "))
	lines = append(lines, "", "[Rule]", "FINAL,PROXY", "")
	return strings.Join(lines, "\n"), nil
}
