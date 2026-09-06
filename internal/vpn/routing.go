package vpn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type commonRouting struct {
	DomainStrategy string        `json:"domainStrategy,omitempty"`
	Rules          []routingRule `json:"rules"`
}

type routingRule struct {
	Type        string   `json:"type"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        string   `json:"port,omitempty"`
	Network     string   `json:"network,omitempty"`
	Protocol    []string `json:"protocol,omitempty"`
	OutboundTag string   `json:"outboundTag,omitempty"`
}

// parseCommonRouting 解析唯一的公共 Xray routing 配置。
// 输入：content 是 common-routing.json 原文。
// 输出：返回可用于多客户端转换的结构化规则；空内容表示没有公共规则。
// 副作用：无。
func parseCommonRouting(content []byte) (commonRouting, error) {
	// 1. 空文件保持现有订阅兼容，并使用稳定的 Xray 默认策略。
	routing := commonRouting{DomainStrategy: "IPIfNonMatch", Rules: []routingRule{}}
	if strings.TrimSpace(string(content)) == "" {
		return routing, nil
	}

	// 2. 严格解码单个 JSON 对象，避免 AI 修改时拼入未支持字段后静默失效。
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&routing); err != nil {
		return commonRouting{}, fmt.Errorf("解析公共 VPN 分流规则: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return commonRouting{}, fmt.Errorf("公共 VPN 分流规则只能包含一个 JSON 对象")
		}
		return commonRouting{}, fmt.Errorf("读取公共 VPN 分流规则尾部: %w", err)
	}
	if routing.DomainStrategy == "" {
		routing.DomainStrategy = "IPIfNonMatch"
	}
	if routing.Rules == nil {
		routing.Rules = []routingRule{}
	}

	// 3. 在读取订阅之前校验每条规则的可转换边界。
	for index, rule := range routing.Rules {
		if strings.ToLower(strings.TrimSpace(rule.Type)) != "field" {
			return commonRouting{}, fmt.Errorf("公共 VPN 分流规则第 %d 条类型 %q 不支持", index+1, rule.Type)
		}
		if _, err := routingTarget(rule.OutboundTag, "PROXY"); err != nil {
			return commonRouting{}, fmt.Errorf("公共 VPN 分流规则第 %d 条: %w", index+1, err)
		}
		if err := validateRoutingMatcher(rule); err != nil {
			return commonRouting{}, fmt.Errorf("公共 VPN 分流规则第 %d 条: %w", index+1, err)
		}
	}
	return routing, nil
}

// JSON 返回规范化的公共 routing JSON。
// 输入：无。
// 输出：返回带缩进和换行的 JSON 文本。
// 副作用：无。
func (r commonRouting) JSON() (string, error) {
	// 1. 确保空规则序列编码为 [] 而不是 null。
	if r.Rules == nil {
		r.Rules = []routingRule{}
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("编码公共 VPN 分流规则: %w", err)
	}
	return string(encoded) + "\n", nil
}

// RuleLines 把 Xray field 规则转换为目标客户端的规则行。
// 输入：client 是 clash、surge 或 shadowrocket；proxyPolicy 是代理组名。
// 输出：返回保持原顺序的客户端规则行。
// 副作用：无。
func (r commonRouting) RuleLines(client, proxyPolicy string) ([]string, error) {
	// 1. 限定目标客户端，避免把未定义的转换结果发送出去。
	client = strings.ToLower(strings.TrimSpace(client))
	if client != "clash" && client != "surge" && client != "shadowrocket" {
		return nil, fmt.Errorf("客户端 %q 不支持公共 VPN 分流规则", client)
	}
	lines := make([]string, 0, len(r.Rules))
	for index, rule := range r.Rules {
		target, err := routingTarget(rule.OutboundTag, proxyPolicy)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条规则: %w", index+1, err)
		}
		if len(rule.Protocol) > 0 {
			return nil, fmt.Errorf("第 %d 条规则的 protocol 条件无法跨客户端转换", index+1)
		}
		matchers := 0
		if len(rule.Domain) > 0 {
			matchers++
		}
		if len(rule.IP) > 0 {
			matchers++
		}
		if strings.TrimSpace(rule.Port) != "" {
			matchers++
		}
		if strings.TrimSpace(rule.Network) != "" {
			matchers++
		}
		if matchers != 1 {
			return nil, fmt.Errorf("第 %d 条规则必须且只能包含一种匹配条件", index+1)
		}

		switch {
		case len(rule.Domain) > 0:
			for _, domain := range rule.Domain {
				line, lineErr := domainRuleLine(domain, target)
				if lineErr != nil {
					return nil, fmt.Errorf("第 %d 条规则: %w", index+1, lineErr)
				}
				lines = append(lines, line)
			}
		case len(rule.IP) > 0:
			for _, ip := range rule.IP {
				line, lineErr := ipRuleLine(ip, target)
				if lineErr != nil {
					return nil, fmt.Errorf("第 %d 条规则: %w", index+1, lineErr)
				}
				lines = append(lines, line)
			}
		case strings.TrimSpace(rule.Port) != "":
			port, parseErr := strconv.Atoi(strings.TrimSpace(rule.Port))
			if parseErr != nil || port <= 0 || port > 65535 {
				return nil, fmt.Errorf("第 %d 条规则端口无效", index+1)
			}
			matcher := "DST-PORT"
			if client == "surge" || client == "shadowrocket" {
				matcher = "DEST-PORT"
			}
			lines = append(lines, fmt.Sprintf("%s,%d,%s", matcher, port, target))
		default:
			network := strings.ToUpper(strings.TrimSpace(rule.Network))
			if network != "TCP" && network != "UDP" {
				return nil, fmt.Errorf("第 %d 条规则 network %q 不支持", index+1, rule.Network)
			}
			lines = append(lines, fmt.Sprintf("NETWORK,%s,%s", network, target))
		}
	}
	return lines, nil
}

// replaceRuleSection 替换 Surge/Shadowrocket 的一个 INI 风格规则区段。
// 输入：content 是原配置，section 是不含方括号的区段名，rules 是新规则行。
// 输出：返回保留其他区段的配置正文。
// 副作用：无。
func replaceRuleSection(content, section string, rules []string) (string, error) {
	section = strings.TrimSpace(section)
	if section == "" {
		return "", fmt.Errorf("配置区段名称不能为空")
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	header := "[" + section + "]"
	start := -1
	for index, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), header) {
			start = index
			break
		}
	}
	if start >= 0 {
		end := len(lines)
		for index := start + 1; index < len(lines); index++ {
			if isConfigSectionHeader(lines[index]) {
				end = index
				break
			}
		}
		result := append([]string{}, lines[:start+1]...)
		result = append(result, rules...)
		if end < len(lines) && len(result) > 0 && result[len(result)-1] != "" {
			result = append(result, "")
		}
		result = append(result, lines[end:]...)
		return strings.Join(result, "\n"), nil
	}

	base := strings.TrimRight(normalized, "\n")
	if base != "" {
		base += "\n\n"
	}
	result := base + header + "\n" + strings.Join(rules, "\n")
	if strings.HasSuffix(normalized, "\n") {
		result += "\n"
	}
	return result, nil
}

func validateRoutingMatcher(rule routingRule) error {
	matchers := 0
	if len(rule.Domain) > 0 {
		matchers++
	}
	if len(rule.IP) > 0 {
		matchers++
	}
	if strings.TrimSpace(rule.Port) != "" {
		matchers++
	}
	if strings.TrimSpace(rule.Network) != "" {
		matchers++
	}
	if len(rule.Protocol) > 0 {
		return fmt.Errorf("protocol 条件无法跨客户端转换")
	}
	if matchers != 1 {
		return fmt.Errorf("必须且只能包含一种匹配条件")
	}
	return nil
}

func routingTarget(value, proxyPolicy string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "proxy":
		if strings.TrimSpace(proxyPolicy) == "" {
			return "", fmt.Errorf("proxy 规则缺少代理组")
		}
		return strings.TrimSpace(proxyPolicy), nil
	case "direct":
		return "DIRECT", nil
	case "block":
		return "REJECT", nil
	default:
		return "", fmt.Errorf("outboundTag %q 不支持", value)
	}
}

func domainRuleLine(value, target string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("domain 条件不能为空")
	}
	matcher, domain := "DOMAIN-SUFFIX", value
	if parts := strings.SplitN(value, ":", 2); len(parts) == 2 {
		domain = strings.TrimSpace(parts[1])
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "domain":
			matcher = "DOMAIN-SUFFIX"
		case "full":
			matcher = "DOMAIN"
		case "keyword":
			matcher = "DOMAIN-KEYWORD"
		case "regexp":
			matcher = "DOMAIN-REGEX"
		default:
			return "", fmt.Errorf("domain 条件 %q 不支持", value)
		}
	}
	if domain == "" {
		return "", fmt.Errorf("domain 条件不能为空")
	}
	return matcher + "," + domain + "," + target, nil
}

func ipRuleLine(value, target string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("ip 条件不能为空")
	}
	if parts := strings.SplitN(value, ":", 2); len(parts) == 2 && strings.EqualFold(parts[0], "geoip") {
		if parts[1] == "" {
			return "", fmt.Errorf("geoip 条件不能为空")
		}
		return "GEOIP," + strings.ToUpper(strings.TrimSpace(parts[1])) + "," + target, nil
	}
	ip, _, err := net.ParseCIDR(value)
	if err != nil {
		return "", fmt.Errorf("ip 条件 %q 不是有效 CIDR", value)
	}
	matcher := "IP-CIDR6"
	if ip.To4() != nil {
		matcher = "IP-CIDR"
	}
	return matcher + "," + value + "," + target, nil
}

func isConfigSectionHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) > 2 && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
}

func mergeClashRouting(content []byte, routing commonRouting) (string, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return "", fmt.Errorf("解析 Clash 配置: %w", err)
	}
	policy := clashProxyPolicy(document)
	lines, err := routing.RuleLines("clash", policy)
	if err != nil {
		return "", err
	}
	document["rules"] = lines
	if _, exists := document["proxy-groups"]; !exists {
		document["proxy-groups"] = []map[string]any{{
			"name": "PROXY", "type": "select", "proxies": clashProxyNames(document),
		}}
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("编码 Clash 公共分流规则: %w", err)
	}
	return string(encoded), nil
}

func mergeTextRouting(content, client string, routing commonRouting) (string, error) {
	policy := iniProxyPolicy(content)
	lines, err := routing.RuleLines(client, policy)
	if err != nil {
		return "", err
	}
	return replaceRuleSection(content, "Rule", lines)
}

func clashProxyPolicy(document map[string]any) string {
	first := ""
	for _, group := range dynamicList(document["proxy-groups"]) {
		name := dynamicString(dynamicMapValue(group, "name"))
		if name == "" {
			continue
		}
		if strings.EqualFold(name, "PROXY") {
			return name
		}
		if first == "" && !isTerminalPolicy(name) {
			first = name
		}
	}
	if first != "" {
		return first
	}
	return "PROXY"
}

func clashProxyNames(document map[string]any) []string {
	names := make([]string, 0)
	for _, proxy := range dynamicList(document["proxies"]) {
		if name := dynamicString(dynamicMapValue(proxy, "name")); name != "" {
			names = append(names, name)
		}
	}
	names = append(names, "DIRECT")
	return names
}

func iniProxyPolicy(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inProxyGroup := false
	first := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isConfigSectionHeader(trimmed) {
			inProxyGroup = strings.EqualFold(trimmed, "[Proxy Group]")
			continue
		}
		if !inProxyGroup {
			continue
		}
		name, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if strings.EqualFold(name, "PROXY") {
			return name
		}
		if first == "" && !isTerminalPolicy(name) {
			first = name
		}
	}
	if first != "" {
		return first
	}
	return "PROXY"
}

func isTerminalPolicy(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "DIRECT") || strings.EqualFold(strings.TrimSpace(value), "REJECT")
}

func dynamicList(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = typed[index]
		}
		return result
	default:
		return nil
	}
}

func dynamicMapValue(value any, key string) any {
	if typed, ok := value.(map[string]any); ok {
		return typed[key]
	}
	return nil
}

func dynamicString(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}
