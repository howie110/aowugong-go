package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// OpenILinkClient 使用 Hub Bot API 发送微信文本、图片和文件。
type OpenILinkClient struct {
	config     config.OpenILink
	httpClient *http.Client
}

// NewOpenILinkClient 创建 OpeniLink Hub 客户端。
// 输入：cfg 提供 Hub 地址、App Token 和默认接收人，httpClient 提供连接复用。
// 输出：返回可并发复用的客户端。
// 副作用：无，不发起网络请求。
func NewOpenILinkClient(cfg config.OpenILink, httpClient *http.Client) *OpenILinkClient {
	// 1. 应用默认超时并清理 Hub 基础地址。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	cfg.HubURL = strings.TrimRight(strings.TrimSpace(cfg.HubURL), "/")
	return &OpenILinkClient{config: cfg, httpClient: httpClient}
}

// Configured 返回 Hub 地址和 App Token 是否均已配置。
// 输入：无。
// 输出：配置完整时返回 true。
// 副作用：无。
func (c *OpenILinkClient) Configured() bool {
	// 1. 只返回状态，不暴露 App Token。
	return c != nil && c.config.HubURL != "" && strings.TrimSpace(c.config.AppToken) != ""
}

// SendText 发送一条微信文本消息。
// 输入：ctx 控制请求，content 是正文，to 可覆盖默认接收人。
// 输出：返回 Hub 原始 JSON；配置或发送失败时返回错误。
// 副作用：调用 OpeniLink Hub 发送微信消息。
func (c *OpenILinkClient) SendText(ctx context.Context, content, to string) (map[string]any, error) {
	// 1. 校验并组装文本消息。
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}
	payload := map[string]any{"type": "text", "content": content}
	c.applyRecipient(payload, to)

	// 2. 通过唯一发送入口提交消息。
	return c.postMessage(ctx, payload)
}

// SendFileURL 发送一个 URL 文件。
// 输入：ctx 控制请求，url 和 filename 描述文件，to 可覆盖接收人。
// 输出：返回 Hub 原始 JSON；发送失败时返回错误。
// 副作用：调用 OpeniLink Hub 发送微信文件。
func (c *OpenILinkClient) SendFileURL(ctx context.Context, url, filename, to string) (map[string]any, error) {
	// 1. 复用 URL 媒体发送入口并指定文件类型。
	return c.sendMediaURL(ctx, "file", url, filename, to)
}

// SendImageURL 发送一个 URL 图片。
// 输入：ctx 控制请求，url 和 filename 描述图片，to 可覆盖接收人。
// 输出：返回 Hub 原始 JSON；发送失败时返回错误。
// 副作用：调用 OpeniLink Hub 发送微信图片。
func (c *OpenILinkClient) SendImageURL(ctx context.Context, url, filename, to string) (map[string]any, error) {
	// 1. 复用 URL 媒体发送入口并指定图片类型。
	return c.sendMediaURL(ctx, "image", url, filename, to)
}

// SendFile 读取并发送一个本地文件。
// 输入：ctx 控制请求，path 是本地文件，to 可覆盖接收人。
// 输出：返回 Hub 原始 JSON；读取或发送失败时返回错误。
// 副作用：读取本地文件并调用 OpeniLink Hub 发送微信文件。
func (c *OpenILinkClient) SendFile(ctx context.Context, path, to string) (map[string]any, error) {
	// 1. 复用本地媒体入口并指定文件类型。
	return c.sendLocalMedia(ctx, "file", path, to)
}

// SendImage 读取并发送一个本地图片。
// 输入：ctx 控制请求，path 是本地图片，to 可覆盖接收人。
// 输出：返回 Hub 原始 JSON；读取或发送失败时返回错误。
// 副作用：读取本地图片并调用 OpeniLink Hub 发送微信图片。
func (c *OpenILinkClient) SendImage(ctx context.Context, path, to string) (map[string]any, error) {
	// 1. 复用本地媒体入口并指定图片类型。
	return c.sendLocalMedia(ctx, "image", path, to)
}

// sendMediaURL 发送 URL 形式的图片或文件。
// 输入：ctx 控制请求，mediaType 是 image/file，url、filename 和 to 描述消息。
// 输出：返回 Hub 原始 JSON；参数或发送失败时返回错误。
// 副作用：调用 OpeniLink Hub 发送微信媒体。
func (c *OpenILinkClient) sendMediaURL(ctx context.Context, mediaType, url, filename, to string) (map[string]any, error) {
	// 1. 校验媒体地址和文件名并构造请求体。
	url = strings.TrimSpace(url)
	filename = strings.TrimSpace(filename)
	if url == "" || filename == "" {
		return nil, fmt.Errorf("媒体 URL 和文件名不能为空")
	}
	payload := map[string]any{"type": mediaType, "url": url, "filename": filename}
	c.applyRecipient(payload, to)

	// 2. 通过唯一发送入口提交消息。
	return c.postMessage(ctx, payload)
}

// sendLocalMedia 读取本地媒体并以 base64 发送。
// 输入：ctx 控制请求，mediaType 是 image/file，path 是路径，to 是接收人。
// 输出：返回 Hub 原始 JSON；文件或发送失败时返回错误。
// 副作用：读取本地文件并调用 OpeniLink Hub。
func (c *OpenILinkClient) sendLocalMedia(ctx context.Context, mediaType, path, to string) (map[string]any, error) {
	// 1. 打开普通文件并限制单个微信附件为三十二兆字节。
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("打开通知附件 %s: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("读取通知附件信息 %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return nil, fmt.Errorf("通知附件必须是小于等于 32MB 的普通文件")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取通知附件 %s: %w", path, err)
	}

	// 2. 组装 base64 媒体并提交到唯一发送入口。
	payload := map[string]any{
		"type": mediaType, "base64": base64.StdEncoding.EncodeToString(data), "filename": info.Name(),
	}
	c.applyRecipient(payload, to)
	return c.postMessage(ctx, payload)
}

// applyRecipient 应用显式接收人或客户端默认接收人。
// 输入：payload 是待发送消息，to 是可选显式接收人。
// 输出：无。
// 副作用：可能向 payload 写入 to 字段。
func (c *OpenILinkClient) applyRecipient(payload map[string]any, to string) {
	// 1. 显式值优先，随后使用配置默认值，空值则省略字段。
	recipient := strings.TrimSpace(to)
	if recipient == "" {
		recipient = strings.TrimSpace(c.config.DefaultTo)
	}
	if recipient != "" {
		payload["to"] = recipient
	}
}

// postMessage 提交 JSON 消息并校验 HTTP 与业务状态。
// 输入：ctx 控制请求，payload 是完整消息。
// 输出：返回 Hub JSON；配置、HTTP、JSON 或业务失败时返回错误。
// 副作用：调用 OpeniLink Hub。
func (c *OpenILinkClient) postMessage(ctx context.Context, payload map[string]any) (map[string]any, error) {
	// 1. 校验配置并创建带 Bearer Token 的 JSON 请求。
	if !c.Configured() {
		return nil, fmt.Errorf("未配置 OPENILINK_HUB_URL 或 OPENILINK_APP_TOKEN")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 OpeniLink 消息: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.HubURL+"/bot/v1/message/send", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 OpeniLink 请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.config.AppToken)
	request.Header.Set("Content-Type", "application/json")

	// 2. 执行请求并限制响应体大小。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求 OpeniLink Hub: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 OpeniLink 响应: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("OpeniLink 接口返回非 JSON: %w", err)
	}

	// 3. 把 HTTP 和 ok=false 统一转换成可传播错误。
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OpeniLink 接口返回 HTTP %d: %s", response.StatusCode, openILinkMessage(result))
	}
	if ok, exists := result["ok"].(bool); exists && !ok {
		return nil, fmt.Errorf("OpeniLink 接口返回失败: %s", openILinkMessage(result))
	}
	return result, nil
}

// openILinkMessage 从 Hub 响应提取错误或提示文本。
// 输入：result 是已解码 JSON。
// 输出：优先返回 error、message，否则返回通用说明。
// 副作用：无。
func openILinkMessage(result map[string]any) string {
	// 1. 依次选择 Hub 常用错误字段。
	for _, key := range []string{"error", "message"} {
		if value, ok := result[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "上游未返回错误说明"
}
