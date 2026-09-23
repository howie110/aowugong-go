package vpn

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Build 读取指定资源并生成全部可用客户端配置。
// 输入：profileCode 是资源编码。
// 输出：返回按格式索引的配置；资源缺失或文件无效时返回错误。
// 副作用：读取 storage/private/vpn 下对应文件。
func (c *SourceCatalog) Build(profileCode string) (map[string]ConfigContent, error) {
	// 1. 严格清理资源编码，阻止路径穿越和模糊匹配。
	profileCode = strings.ToLower(strings.TrimSpace(profileCode))
	if !validProfileCode(profileCode) {
		return nil, ErrProfileNotFound
	}
	files, err := c.sourceFiles()
	if err != nil {
		return nil, err
	}
	matched := make(map[string]string)
	for _, file := range files {
		kind, code, ok := parseSourceFilename(file.Name())
		if ok && code == profileCode {
			matched[kind] = filepath.Join(c.directory, file.Name())
		}
	}
	if len(matched) == 0 {
		return nil, ErrProfileNotFound
	}
	routing, err := c.loadCommonRouting()
	if err != nil {
		return nil, err
	}
	routingBody, err := routing.JSON()
	if err != nil {
		return nil, err
	}

	// 2. 优先保留各客户端原生配置，避免跨格式转换丢失 TLS 安全参数。
	configs := map[string]ConfigContent{
		"routing": {
			ContentType: "application/json; charset=utf-8",
			Filename:    profileCode + "-routing.json",
			Body:        routingBody,
		},
	}
	clashPath := matched["clash"]
	if clashPath == "" {
		clashPath = matched["flclash"]
	}
	if clashPath != "" {
		content, readErr := readPrivateSource(clashPath)
		if readErr != nil {
			return nil, readErr
		}
		body := string(content)
		if len(routing.Rules) > 0 {
			body, err = mergeClashRouting(content, routing)
			if err != nil {
				return nil, fmt.Errorf("合并 %s Clash 公共分流规则: %w", profileCode, err)
			}
		}
		configs["clash"] = ConfigContent{
			ContentType: "text/yaml; charset=utf-8", Filename: profileCode + "-clash.yaml", Body: body,
		}
	}
	for _, exact := range []struct {
		kind        string
		format      string
		contentType string
		extension   string
	}{
		{kind: "shadowrocket", format: "shadowrocket", contentType: "text/plain; charset=utf-8", extension: ".conf"},
		{kind: "surge", format: "surge", contentType: "text/plain; charset=utf-8", extension: ".conf"},
	} {
		path := matched[exact.kind]
		if path == "" {
			continue
		}
		content, readErr := readPrivateSource(path)
		if readErr != nil {
			return nil, readErr
		}
		body := string(content)
		if len(routing.Rules) > 0 {
			body, err = mergeTextRouting(body, exact.format, routing)
			if err != nil {
				return nil, fmt.Errorf("合并 %s %s 公共分流规则: %w", profileCode, exact.format, err)
			}
		}
		configs[exact.format] = ConfigContent{
			ContentType: exact.contentType, Filename: profileCode + "-" + exact.format + exact.extension, Body: body,
		}
	}
	if matched["v2rayn"] != "" {
		content, readErr := readPrivateSource(matched["v2rayn"])
		if readErr != nil {
			return nil, readErr
		}
		v2rayContent, convertErr := xraySubscription(content, profileCode)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Xray 节点: %w", profileCode, convertErr)
		}
		configs["v2ray"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-v2ray.txt", Body: v2rayContent,
		}
	} else if clashConfig, exists := configs["clash"]; exists {
		v2rayContent, convertErr := clashSubscription([]byte(clashConfig.Body), profileCode)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Clash 节点: %w", profileCode, convertErr)
		}
		configs["v2ray"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-v2ray.txt", Body: v2rayContent,
		}
	}

	// 3. 从标准 VMess 订阅补齐缺少的客户端格式，保证每个资源固定输出四种资源。
	v2rayConfig, exists := configs["v2ray"]
	if !exists {
		return nil, fmt.Errorf("资源 %s 无法生成标准节点订阅", profileCode)
	}
	if _, exists := configs["clash"]; !exists {
		body, convertErr := vmessSubscriptionToClash(v2rayConfig.Body, profileCode)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Clash 配置: %w", profileCode, convertErr)
		}
		if len(routing.Rules) > 0 {
			body, convertErr = mergeClashRouting([]byte(body), routing)
			if convertErr != nil {
				return nil, fmt.Errorf("合并 %s Clash 公共分流规则: %w", profileCode, convertErr)
			}
		}
		configs["clash"] = ConfigContent{
			ContentType: "text/yaml; charset=utf-8", Filename: profileCode + "-clash.yaml", Body: body,
		}
	}
	if _, exists := configs["shadowrocket"]; !exists {
		body := v2rayConfig.Body
		if len(routing.Rules) > 0 {
			body, err = vmessSubscriptionToSurge(v2rayConfig.Body)
			if err != nil {
				return nil, fmt.Errorf("转换 %s Shadowrocket 配置: %w", profileCode, err)
			}
			body, err = mergeTextRouting(body, "shadowrocket", routing)
			if err != nil {
				return nil, fmt.Errorf("合并 %s Shadowrocket 公共分流规则: %w", profileCode, err)
			}
		}
		configs["shadowrocket"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-shadowrocket.txt", Body: body,
		}
	}
	if _, exists := configs["surge"]; !exists {
		body, convertErr := vmessSubscriptionToSurge(v2rayConfig.Body)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Surge 配置: %w", profileCode, convertErr)
		}
		if len(routing.Rules) > 0 {
			body, convertErr = mergeTextRouting(body, "surge", routing)
			if convertErr != nil {
				return nil, fmt.Errorf("合并 %s Surge 公共分流规则: %w", profileCode, convertErr)
			}
		}
		configs["surge"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-surge.conf", Body: body,
		}
	}
	return configs, nil
}
