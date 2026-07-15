package monitoring

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/howiedata/aowugong-go/internal/config"
)

const (
	defaultBlogURL  = "https://www.aowugong.top/"
	defaultMovieURL = "https://movie.aowugong.top/"
)

var nonCodeCharacters = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// BuildTargets 构建当前有效的服务监控清单。
// 输入：cfg 是外部服务和额外目标配置。
// 输出：返回按 code 去重的目标列表。
// 副作用：无。
func BuildTargets(cfg config.Clients) []Target {
	// 1. 加入两个固定公开站点。
	targets := []Target{
		{Code: "aowugong-blog", Name: "howie110/astro-theme-retypeset", URL: defaultBlogURL, Description: textPointer("个人博客站点")},
		{Code: "movie-carousel", Name: "howie110/Movie-Images", URL: defaultMovieURL, Description: textPointer("电影画面轮播页面")},
	}

	// 2. 按配置加入 WeChatRSS 登录状态和 OpeniLink 发送能力目标。
	wechatURL := strings.TrimSpace(cfg.WeChatRSSMonitorURL)
	if wechatURL == "" && strings.TrimSpace(cfg.ArticleRSSURL) != "" {
		wechatURL = endpointURL(cfg.ArticleRSSURL, "/api/admin/status")
	}
	if wechatURL != "" {
		targets = append(targets, Target{
			Code: "wechat-rss", Name: "tmwgsicp/wechat-download-api", URL: wechatURL,
			Description: textPointer("微信登录状态与 RSS 聚合服务"),
		})
	}
	openILinkURL := strings.TrimSpace(cfg.OpenILink.MonitorURL)
	if openILinkURL == "" {
		openILinkURL = strings.TrimSpace(cfg.OpenILink.HubURL)
	}
	if openILinkURL != "" {
		targets = append(targets, Target{
			Code: "openilink-hub", Name: "openilink/openilink-hub", URL: endpointURL(openILinkURL, "/bot/v1/message/send"),
			Description: textPointer("静默验证微信通知链路"),
		})
	}

	// 3. 合并有效额外目标并保留 code 第一次出现的目标。
	targets = append(targets, parseExtraTargets(cfg.ServiceMonitorTargets)...)
	seen := make(map[string]struct{}, len(targets))
	result := make([]Target, 0, len(targets))
	for _, target := range targets {
		if _, exists := seen[target.Code]; exists {
			continue
		}
		seen[target.Code] = struct{}{}
		result = append(result, target)
	}
	return result
}

// parseExtraTargets 解析 SERVICE_MONITOR_TARGETS JSON。
// 输入：raw 是 JSON 数组文本。
// 输出：返回清洗后的有效目标；非法 JSON 返回空列表。
// 副作用：无。
func parseExtraTargets(raw string) []Target {
	// 1. 解码宽松字段，非法配置不阻止应用启动。
	var values []struct {
		Code        string `json:"code"`
		Name        string `json:"name"`
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &values) != nil {
		return []Target{}
	}

	// 2. 清理必填字段、长度和缺省 code。
	targets := make([]Target, 0, len(values))
	for _, value := range values {
		name := truncateRunes(strings.TrimSpace(value.Name), 120)
		address := truncateRunes(strings.TrimSpace(value.URL), 500)
		if name == "" || address == "" {
			continue
		}
		code := truncateRunes(strings.TrimSpace(value.Code), 80)
		if code == "" {
			code = slugify(name)
		}
		description := truncateRunes(strings.TrimSpace(value.Description), 200)
		targets = append(targets, Target{Code: code, Name: name, URL: address, Description: textPointer(description)})
	}
	return targets
}

// endpointURL 把任意同源地址转换为指定绝对路径。
// 输入：base 是基础地址，path 是目标路径。
// 输出：返回同 scheme/host 的目标 URL；解析失败时返回原地址。
// 副作用：无。
func endpointURL(base, path string) string {
	// 1. 解析地址并仅替换 path、query 和 fragment。
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return base
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// slugify 把展示名称转换为稳定监控 code。
// 输入：value 是展示名称。
// 输出：返回小写连字符 code。
// 副作用：无。
func slugify(value string) string {
	// 1. 替换连续非字母数字字符并清理边界。
	code := strings.Trim(nonCodeCharacters.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
	if code == "" {
		return "service"
	}
	return code
}

// truncateRunes 按字符数截断文本。
// 输入：value 是文本，limit 是最大字符数。
// 输出：返回不超过限制的文本。
// 副作用：无。
func truncateRunes(value string, limit int) string {
	// 1. 使用 rune 避免截断中文 UTF-8 字节。
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// textPointer 把非空文本转换为字符串指针。
// 输入：value 是可选文本。
// 输出：空值返回 nil。
// 副作用：无。
func textPointer(value string) *string {
	// 1. 保留 JSON null 和空文本的区别。
	if value == "" {
		return nil
	}
	return &value
}

// ValidateWeChatRSSLoginPayload 校验 WeChatRSS 登录状态载荷。
// 输入：payload 是管理接口 JSON 字典。
// 输出：健康返回空字符串，异常返回可读说明。
// 副作用：无。
func ValidateWeChatRSSLoginPayload(payload map[string]any) string {
	// 1. 只有两个登录字段为真且未过期才视为健康。
	authenticated, _ := payload["authenticated"].(bool)
	loggedIn, _ := payload["loggedIn"].(bool)
	expired, _ := payload["isExpired"].(bool)
	if authenticated && loggedIn && !expired {
		return ""
	}
	status := strings.TrimSpace(toText(payload["status"]))
	if status == "" {
		status = "未登录或登录已过期"
	}
	account := strings.TrimSpace(toText(payload["account"]))
	if account == "" {
		account = strings.TrimSpace(toText(payload["nickname"]))
	}
	return strings.TrimSpace("WeChatRSS 登录异常：" + account + " " + status)
}

// toText 把外部 JSON 标量转换为文本。
// 输入：value 是任意值。
// 输出：字符串值原样返回，其他类型返回空字符串。
// 副作用：无。
func toText(value any) string {
	// 1. 监控错误字段只接受字符串，避免展示 map 内存格式。
	text, _ := value.(string)
	return text
}
