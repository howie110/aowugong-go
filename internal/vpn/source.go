package vpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxSourceFileBytes = 2 * 1024 * 1024

var formatNames = map[string]string{
	"clash":        "Clash / FlClash",
	"v2ray":        "v2rayN / v2rayNG",
	"shadowrocket": "Shadowrocket",
	"surge":        "Surge",
}

var requiredFormats = []string{"clash", "shadowrocket", "surge", "v2ray"}

// SourceCatalog 从私有目录发现并转换 VPN 客户端配置。
type SourceCatalog struct {
	directory string
}

// NewSourceCatalog 创建 VPN 私有资源目录读取器。
// 输入：directory 是只包含私有 VPN 文件的目录。
// 输出：返回资源目录读取器。
// 副作用：无，不读取目录。
func NewSourceCatalog(directory string) *SourceCatalog {
	// 1. 保存清理后的目录路径，后续只读取其直接子文件。
	return &SourceCatalog{directory: filepath.Clean(directory)}
}

// Profiles 返回当前目录可用的资源和客户端格式。
// 输入：无。
// 输出：返回按资源编码排序的列表；目录缺失时返回空列表。
// 副作用：读取私有目录文件名，不读取节点内容。
func (c *SourceCatalog) Profiles() ([]Profile, error) {
	// 1. 收集受支持文件，并把同一资源的多种客户端格式归组。
	files, err := c.sourceFiles()
	if err != nil {
		return nil, err
	}
	grouped := make(map[string]struct{})
	for _, file := range files {
		_, profileCode, ok := parseSourceFilename(file.Name())
		if !ok {
			continue
		}
		grouped[profileCode] = struct{}{}
	}

	// 2. 稳定排序资源和格式，保证页面不会因目录遍历顺序跳动。
	profiles := make([]Profile, 0, len(grouped))
	for code := range grouped {
		formatList := make([]Format, 0, len(requiredFormats))
		for _, formatCode := range requiredFormats {
			formatList = append(formatList, Format{Code: formatCode, Name: formatNames[formatCode]})
		}
		profiles = append(profiles, Profile{Code: code, Name: profileDisplayName(code), Formats: formatList})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Code < profiles[j].Code })
	return profiles, nil
}

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

	// 2. 优先使用原生客户端配置，Clash 同时作为通用链接转换来源。
	configs := make(map[string]ConfigContent)
	clashPath := matched["clash"]
	if clashPath == "" {
		clashPath = matched["flclash"]
	}
	if clashPath != "" {
		content, readErr := readPrivateSource(clashPath)
		if readErr != nil {
			return nil, readErr
		}
		configs["clash"] = ConfigContent{
			ContentType: "text/yaml; charset=utf-8", Filename: profileCode + "-clash.yaml", Body: string(content),
		}
		v2rayContent, convertErr := clashSubscription(content, profileCode)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Clash 节点: %w", profileCode, convertErr)
		}
		configs["v2ray"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-v2ray.txt", Body: v2rayContent,
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
		configs[exact.format] = ConfigContent{
			ContentType: exact.contentType, Filename: profileCode + "-" + exact.format + exact.extension, Body: string(content),
		}
	}
	if _, exists := configs["v2ray"]; !exists && matched["v2rayn"] != "" {
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
		configs["clash"] = ConfigContent{
			ContentType: "text/yaml; charset=utf-8", Filename: profileCode + "-clash.yaml", Body: body,
		}
	}
	if _, exists := configs["shadowrocket"]; !exists {
		configs["shadowrocket"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-shadowrocket.txt", Body: v2rayConfig.Body,
		}
	}
	if _, exists := configs["surge"]; !exists {
		body, convertErr := vmessSubscriptionToSurge(v2rayConfig.Body)
		if convertErr != nil {
			return nil, fmt.Errorf("转换 %s Surge 配置: %w", profileCode, convertErr)
		}
		configs["surge"] = ConfigContent{
			ContentType: "text/plain; charset=utf-8", Filename: profileCode + "-surge.conf", Body: body,
		}
	}
	return configs, nil
}

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

// parseIntDefault 把可选十进制文本转换为整数。
// 输入：value 是十进制文本，fallback 是空值或无效值回退。
// 输出：返回解析整数或 fallback。
// 副作用：无。
func parseIntDefault(value string, fallback int) int {
	// 1. 解析失败时保持调用方指定默认值。
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// sourceFiles 返回私有目录中的普通文件。
// 输入：无。
// 输出：目录缺失时返回空列表，其他读取错误带上下文返回。
// 副作用：读取私有目录元数据。
func (c *SourceCatalog) sourceFiles() ([]os.DirEntry, error) {
	// 1. 将尚未配置私有目录视为没有资源，而不是应用启动错误。
	files, err := os.ReadDir(c.directory)
	if os.IsNotExist(err) {
		return []os.DirEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 VPN 私有资源目录: %w", err)
	}
	result := make([]os.DirEntry, 0, len(files))
	for _, file := range files {
		if file.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := file.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("读取 VPN 私有文件信息: %w", infoErr)
		}
		if info.Mode().IsRegular() {
			result = append(result, file)
		}
	}
	return result, nil
}

// readPrivateSource 读取大小受限的单个私有配置文件。
// 输入：path 是 SourceCatalog 已匹配的直接子文件。
// 输出：返回完整内容；文件过大或读取失败时返回错误。
// 副作用：读取私有 VPN 文件。
func readPrivateSource(path string) ([]byte, error) {
	// 1. 先检查大小，避免误把大文件推送到远端 KV。
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("检查 VPN 私有配置: %w", err)
	}
	if info.Size() > maxSourceFileBytes {
		return nil, fmt.Errorf("VPN 私有配置超过 %d 字节", maxSourceFileBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 VPN 私有配置: %w", err)
	}
	return content, nil
}

// parseSourceFilename 解析受支持的私有配置文件名。
// 输入：name 是不含目录的文件名。
// 输出：返回来源类型、资源编码和匹配标记。
// 副作用：无。
func parseSourceFilename(name string) (string, string, bool) {
	// 1. 按第一个下划线分开客户端类型和资源编码。
	lowerName := strings.ToLower(strings.TrimSpace(name))
	extension := filepath.Ext(lowerName)
	base := strings.TrimSuffix(lowerName, extension)
	separator := strings.IndexByte(base, '_')
	if separator <= 0 || separator == len(base)-1 {
		return "", "", false
	}
	kind, profileCode := base[:separator], base[separator+1:]
	validKind := kind == "clash" || kind == "flclash" || kind == "v2rayn" || kind == "shadowrocket" || kind == "surge"
	if !validKind || !validProfileCode(profileCode) {
		return "", "", false
	}
	return kind, profileCode, true
}

// formatsForSourceKind 返回一个源文件可以提供的订阅格式。
// 输入：kind 是文件名前缀。
// 输出：返回稳定格式编码列表。
// 副作用：无。
func formatsForSourceKind(kind string) []string {
	// 1. Clash 和 Xray 可以转换为通用 v2ray 链接，其余保持原生格式。
	switch kind {
	case "clash", "flclash":
		return []string{"clash", "v2ray"}
	case "v2rayn":
		return []string{"v2ray"}
	case "shadowrocket":
		return []string{"shadowrocket"}
	case "surge":
		return []string{"surge"}
	default:
		return nil
	}
}

// validProfileCode 判断资源编码能否安全用于文件匹配和 URL 展示。
// 输入：value 是待校验编码。
// 输出：仅小写字母、数字、短横线和下划线组成时返回 true。
// 副作用：无。
func validProfileCode(value string) bool {
	// 1. 拒绝空值、过长值和任何路径字符。
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

// profileDisplayName 把资源编码转换为页面显示名。
// 输入：code 是资源编码。
// 输出：返回不改变语义的大写显示名。
// 副作用：无。
func profileDisplayName(code string) string {
	// 1. 当前私有资源均使用供应方简称，统一大写便于识别。
	return strings.ToUpper(strings.ReplaceAll(code, "_", " "))
}

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

// valueString 把 YAML 动态标量转换为稳定字符串。
// 输入：value 是 YAML 或 JSON 标量。
// 输出：返回无科学计数法的文本；空值返回空字符串。
// 副作用：无。
func valueString(value any) string {
	// 1. 覆盖 YAML 常见标量类型并对其他值使用 fmt。
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// valueBool 把 YAML 动态标量转换为布尔值。
// 输入：value 是 YAML 或 JSON 标量。
// 输出：布尔真或文本 true 时返回 true。
// 副作用：无。
func valueBool(value any) bool {
	// 1. 接受解析器产生的布尔值及常见文本形式。
	if typed, ok := value.(bool); ok {
		return typed
	}
	parsed, _ := strconv.ParseBool(valueString(value))
	return parsed
}

// querySuffix 把非空查询参数编码为 URL 后缀。
// 输入：query 是查询参数。
// 输出：空参数返回空字符串，否则返回问号开头的编码值。
// 副作用：无。
func querySuffix(query url.Values) string {
	// 1. 只在存在参数时添加问号。
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

// firstNonEmpty 返回第一个非空字符串。
// 输入：values 是候选文本。
// 输出：返回清理后的第一个非空值。
// 副作用：无。
func firstNonEmpty(values ...string) string {
	// 1. 保持候选优先级并跳过空白。
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

// mapStringAny 把字符串映射转换为 YAML 动态映射。
// 输入：values 是字符串键值。
// 输出：返回相同数据的 any 映射。
// 副作用：无。
func mapStringAny(values map[string]string) map[string]any {
	// 1. 逐项复制，避免调用方修改原始映射。
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
