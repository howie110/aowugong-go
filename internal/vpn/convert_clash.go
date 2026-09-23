package vpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

type clashDocument struct {
	Proxies []map[string]any `yaml:"proxies"`
}

// clashSubscription 把 Clash 节点转换为 v2rayN/v2rayNG 订阅正文。
// 输入：content 是 Clash YAML，profileCode 用于补充空节点名。
// 输出：返回 Base64 编码的分享链接列表。
// 副作用：无。
func clashSubscription(content []byte, profileCode string) (string, error) {
	// 1. 使用 YAML 解析器读取代理节点，避免按文本行猜测结构。
	var document clashDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		return "", fmt.Errorf("解析 Clash YAML: %w", err)
	}
	links := make([]string, 0, len(document.Proxies))
	for index, proxy := range document.Proxies {
		link, err := clashProxyLink(proxy, fmt.Sprintf("%s-%d", profileCode, index+1))
		if err != nil {
			return "", err
		}
		if link != "" {
			links = append(links, link)
		}
	}
	if len(links) == 0 {
		return "", fmt.Errorf("没有可转换的 Clash 节点")
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), nil
}

// clashProxyLink 把一个 Clash 节点转换为标准分享链接。
// 输入：proxy 是结构化节点，fallbackName 是空名称回退值。
// 输出：返回分享链接；不支持的协议返回空字符串。
// 副作用：无。
func clashProxyLink(proxy map[string]any, fallbackName string) (string, error) {
	// 1. 在读取服务器字段前跳过 direct、block 等非代理出站。
	protocol := strings.ToLower(valueString(proxy["type"]))
	supported := protocol == "trojan" || protocol == "vless" || protocol == "ss" ||
		protocol == "hysteria2" || protocol == "hy2" || protocol == "tuic" || protocol == "vmess"
	if !supported {
		return "", nil
	}

	// 2. 提取代理协议共用的名称、地址和端口。
	name := valueString(proxy["name"])
	if name == "" {
		name = fallbackName
	}
	server, port := valueString(proxy["server"]), valueString(proxy["port"])
	if server == "" || port == "" {
		return "", fmt.Errorf("节点 %s 缺少地址或端口", name)
	}
	endpoint := net.JoinHostPort(server, port)
	fragment := url.PathEscape(name)

	// 3. 按协议生成 v2rayN/v2rayNG 可识别的链接。
	switch protocol {
	case "trojan":
		query := proxyQuery(proxy)
		return "trojan://" + url.QueryEscape(valueString(proxy["password"])) + "@" + endpoint + querySuffix(query) + "#" + fragment, nil
	case "vless":
		query := proxyQuery(proxy)
		query.Set("encryption", "none")
		return "vless://" + url.QueryEscape(valueString(proxy["uuid"])) + "@" + endpoint + querySuffix(query) + "#" + fragment, nil
	case "ss":
		credential := valueString(proxy["cipher"]) + ":" + valueString(proxy["password"])
		return "ss://" + base64.RawURLEncoding.EncodeToString([]byte(credential)) + "@" + endpoint + "#" + fragment, nil
	case "hysteria2", "hy2":
		query := proxyQuery(proxy)
		return "hysteria2://" + url.QueryEscape(valueString(proxy["password"])) + "@" + endpoint + querySuffix(query) + "#" + fragment, nil
	case "tuic":
		query := proxyQuery(proxy)
		credential := url.QueryEscape(valueString(proxy["uuid"])) + ":" + url.QueryEscape(valueString(proxy["password"]))
		return "tuic://" + credential + "@" + endpoint + querySuffix(query) + "#" + fragment, nil
	case "vmess":
		return vmessLink(proxy, name, server, port)
	default:
		return "", nil
	}
}

// vmessLink 把 Clash VMess 节点转换为 v2rayN 兼容分享链接。
// 输入：proxy 是节点，name、server 和 port 是已清洗共用字段。
// 输出：返回 vmess:// Base64 JSON 链接。
// 副作用：无。
func vmessLink(proxy map[string]any, name, server, port string) (string, error) {
	// 1. 从网络和 WebSocket 配置构造 v2rayN 分享模型。
	network := valueString(proxy["network"])
	if network == "" {
		network = "tcp"
	}
	path, host := "", ""
	if options, ok := proxy["ws-opts"].(map[string]any); ok {
		path = valueString(options["path"])
		if headers, headerOK := options["headers"].(map[string]any); headerOK {
			host = valueString(headers["Host"])
			if host == "" {
				host = valueString(headers["host"])
			}
		}
	}
	tlsValue := ""
	if valueBool(proxy["tls"]) {
		tlsValue = "tls"
	}
	payload := map[string]string{
		"v": "2", "ps": name, "add": server, "port": port,
		"id": valueString(proxy["uuid"]), "aid": valueString(proxy["alterId"]),
		"scy": valueString(proxy["cipher"]), "net": network, "type": "none",
		"host": host, "path": path, "tls": tlsValue, "sni": firstNonEmpty(valueString(proxy["servername"]), valueString(proxy["sni"])),
	}
	if payload["aid"] == "" {
		payload["aid"] = "0"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("编码 VMess 节点 %s: %w", name, err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(encoded), nil
}

// proxyQuery 构造分享链接共用的 TLS 和传输参数。
// 输入：proxy 是 Clash 节点。
// 输出：返回已清洗 URL 查询参数。
// 副作用：无。
func proxyQuery(proxy map[string]any) url.Values {
	// 1. 映射常用 TLS、SNI、传输类型和跳过证书检查设置。
	query := make(url.Values)
	if valueBool(proxy["tls"]) || valueString(proxy["sni"]) != "" || valueString(proxy["servername"]) != "" {
		query.Set("security", "tls")
	}
	if sni := firstNonEmpty(valueString(proxy["servername"]), valueString(proxy["sni"])); sni != "" {
		query.Set("sni", sni)
	}
	if valueBool(proxy["skip-cert-verify"]) {
		query.Set("allowInsecure", "1")
		query.Set("insecure", "1")
	}
	network := valueString(proxy["network"])
	if network != "" {
		query.Set("type", network)
	}
	if options, ok := proxy["ws-opts"].(map[string]any); ok {
		if path := valueString(options["path"]); path != "" {
			query.Set("path", path)
		}
		if headers, headerOK := options["headers"].(map[string]any); headerOK {
			if host := firstNonEmpty(valueString(headers["Host"]), valueString(headers["host"])); host != "" {
				query.Set("host", host)
			}
		}
	}
	return query
}
