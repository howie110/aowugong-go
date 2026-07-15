// Package client 提供外部 HTTP/API 客户端。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/howiedata/aowugong-go/internal/config"
)

var ErrWeReadUnavailable = errors.New("微信读书接口不可用")

// WeReadClient 封装微信读书 Agent Gateway 协议。
type WeReadClient struct {
	config config.WeRead
	http   *http.Client
}

// NewWeReadClient 创建微信读书客户端。
// 输入：cfg 是网关、API Key 和 Skill 版本，httpClient 是共享 HTTP 客户端。
// 输出：返回微信读书客户端。
// 副作用：无。
func NewWeReadClient(cfg config.WeRead, httpClient *http.Client) *WeReadClient {
	// 1. 清理网关尾斜杠并保存显式注入的 HTTP 客户端。
	cfg.GatewayURL = strings.TrimRight(cfg.GatewayURL, "/")
	return &WeReadClient{config: cfg, http: httpClient}
}

// Call 调用一个微信读书 Skill API，并在版本升级提示时重试一次。
// 输入：ctx 是调用上下文，apiName 是微信读书 API 名，params 是顶层业务参数。
// 输出：返回 JSON 字典；外部错误包装为 ErrWeReadUnavailable。
// 副作用：最多调用两次微信读书外部 HTTP API。
func (c *WeReadClient) Call(ctx context.Context, apiName string, params map[string]any) (map[string]any, error) {
	// 1. 在网络请求前校验本地配置。
	if strings.TrimSpace(c.config.APIKey) == "" {
		return nil, fmt.Errorf("未配置 WEREAD_API_KEY: %w", ErrWeReadUnavailable)
	}
	if c.http == nil || c.config.GatewayURL == "" || apiName == "" {
		return nil, fmt.Errorf("微信读书客户端配置无效: %w", ErrWeReadUnavailable)
	}

	// 2. 把 Skill 固定字段和业务参数平铺到请求体。
	payload := make(map[string]any, len(params)+2)
	payload["api_name"] = apiName
	payload["skill_version"] = c.config.SkillVersion
	for key, value := range params {
		payload[key] = value
	}
	response, err := c.post(ctx, payload)
	if err != nil {
		return nil, err
	}

	// 3. 服务端给出新版本时只更新请求体并自动重试一次。
	if upgrade, ok := response["upgrade_info"].(map[string]any); ok {
		latestVersion := StringValue(upgrade["latest_version"])
		if latestVersion != "" && latestVersion != StringValue(payload["skill_version"]) {
			payload["skill_version"] = latestVersion
			response, err = c.post(ctx, payload)
			if err != nil {
				return nil, err
			}
		}
	}

	// 4. 转换微信读书业务错误和仍未解决的升级提示。
	if errCode := IntValue(response["errcode"]); response["errcode"] != nil && errCode != 0 {
		message := StringValue(response["errmsg"])
		if message == "" {
			message = StringValue(response["msg"])
		}
		if message == "" {
			message = "微信读书接口返回错误"
		}
		return nil, fmt.Errorf("%s: %w", message, ErrWeReadUnavailable)
	}
	if response["upgrade_info"] != nil {
		return nil, fmt.Errorf("微信读书 Skill 需要升级: %w", ErrWeReadUnavailable)
	}
	return response, nil
}

// post 向微信读书网关发送一次 JSON 请求。
// 输入：ctx 是调用上下文，payload 是完整请求体。
// 输出：返回解码后的 JSON 字典。
// 副作用：调用微信读书外部 HTTP API。
func (c *WeReadClient) post(ctx context.Context, payload map[string]any) (map[string]any, error) {
	// 1. 编码请求体并构造带 Bearer 密钥的请求。
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码微信读书请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.GatewayURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建微信读书请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	request.Header.Set("Content-Type", "application/json")

	// 2. 执行请求并限制响应体大小。
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求微信读书接口: %w: %w", err, ErrWeReadUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("微信读书接口状态 %d: %w", response.StatusCode, ErrWeReadUnavailable)
	}

	// 3. 使用 UseNumber 解码，避免较大整数先转成浮点数。
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("解析微信读书响应: %w: %w", err, ErrWeReadUnavailable)
	}
	return result, nil
}

// IntValue 安全地把外部 JSON 数值或字符串转换为 int。
// 输入：value 是任意 JSON 值。
// 输出：无法转换时返回零。
// 副作用：无。
func IntValue(value any) int {
	// 1. 覆盖 UseNumber、标准数字和字符串三类输入。
	switch typed := value.(type) {
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscan(strings.TrimSpace(typed), &parsed)
		return parsed
	default:
		return 0
	}
}

// StringValue 安全地把外部 JSON 值转换为字符串。
// 输入：value 是任意 JSON 值。
// 输出：字符串原样返回，其他标量格式化，nil 返回空字符串。
// 副作用：无。
func StringValue(value any) string {
	// 1. 避免把 nil 格式化为尖括号文本。
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
