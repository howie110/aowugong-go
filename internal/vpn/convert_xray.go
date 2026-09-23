package vpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type xrayDocument struct {
	Outbounds []struct {
		Protocol       string          `json:"protocol"`
		Tag            string          `json:"tag"`
		Settings       json.RawMessage `json:"settings"`
		StreamSettings struct {
			Network  string `json:"network"`
			Security string `json:"security"`
			TLS      struct {
				ServerName    string `json:"serverName"`
				AllowInsecure bool   `json:"allowInsecure"`
			} `json:"tlsSettings"`
			WS struct {
				Path    string            `json:"path"`
				Headers map[string]string `json:"headers"`
			} `json:"wsSettings"`
		} `json:"streamSettings"`
	} `json:"outbounds"`
}

// xraySubscription 把单节点 Xray 配置转换为 v2rayN/v2rayNG 订阅正文。
// 输入：content 是 Xray JSON，profileCode 用于节点显示名。
// 输出：返回 Base64 编码的分享链接列表。
// 副作用：无。
func xraySubscription(content []byte, profileCode string) (string, error) {
	// 1. 解析出站并只转换实际代理协议。
	var document xrayDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return "", fmt.Errorf("解析 Xray JSON: %w", err)
	}
	links := make([]string, 0)
	for _, outbound := range document.Outbounds {
		link, err := xrayOutboundLink(outbound.Protocol, outbound.Tag, outbound.Settings, outbound.StreamSettings, profileCode)
		if err != nil {
			return "", err
		}
		if link != "" {
			links = append(links, link)
		}
	}
	if len(links) == 0 {
		return "", fmt.Errorf("没有可转换的 Xray 代理出站")
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), nil
}

// xrayOutboundLink 转换一个 Xray 代理出站。
// 输入：protocol、tag、settings 和 stream 描述出站，profileCode 是回退名。
// 输出：返回分享链接；非代理出站返回空字符串。
// 副作用：无。
func xrayOutboundLink(protocol, tag string, settings json.RawMessage, stream struct {
	Network  string `json:"network"`
	Security string `json:"security"`
	TLS      struct {
		ServerName    string `json:"serverName"`
		AllowInsecure bool   `json:"allowInsecure"`
	} `json:"tlsSettings"`
	WS struct {
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
	} `json:"wsSettings"`
}, profileCode string) (string, error) {
	// 1. 读取 Trojan、VMess 和 VLESS 共用的服务器结构。
	var parsed struct {
		Servers []map[string]any `json:"servers"`
		VNext   []struct {
			Address string           `json:"address"`
			Port    int              `json:"port"`
			Users   []map[string]any `json:"users"`
		} `json:"vnext"`
	}
	if err := json.Unmarshal(settings, &parsed); err != nil {
		return "", fmt.Errorf("解析 Xray %s 出站: %w", protocol, err)
	}
	name := firstNonEmpty(tag, profileCode)
	proxy := map[string]any{"type": protocol, "name": name, "network": stream.Network}
	if stream.Security == "tls" {
		proxy["tls"] = true
		proxy["sni"] = stream.TLS.ServerName
		proxy["skip-cert-verify"] = stream.TLS.AllowInsecure
	}
	if stream.Network == "ws" {
		proxy["ws-opts"] = map[string]any{"path": stream.WS.Path, "headers": mapStringAny(stream.WS.Headers)}
	}
	if len(parsed.Servers) > 0 {
		for key, value := range parsed.Servers[0] {
			proxy[key] = value
		}
		// 2. 把 Xray 的 address 字段统一为分享链接生成器使用的 server 字段。
		if proxy["server"] == nil {
			proxy["server"] = proxy["address"]
		}
	}
	if len(parsed.VNext) > 0 {
		proxy["server"], proxy["port"] = parsed.VNext[0].Address, parsed.VNext[0].Port
		if len(parsed.VNext[0].Users) > 0 {
			for key, value := range parsed.VNext[0].Users[0] {
				proxy[key] = value
			}
			if proxy["uuid"] == nil {
				proxy["uuid"] = proxy["id"]
			}
			if proxy["cipher"] == nil {
				proxy["cipher"] = proxy["security"]
			}
		}
	}
	return clashProxyLink(proxy, profileCode)
}
