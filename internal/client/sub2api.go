package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// Sub2APIClient 调用 OpenAI Responses 兼容接口完成单轮文本分析。
type Sub2APIClient struct {
	config     config.Sub2API
	model      string
	httpClient *http.Client
}

type responsesRequest struct {
	Model           string `json:"model"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	Store           bool   `json:"store"`
}

type responsesResponse struct {
	Status string `json:"status"`
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// sub2APIHTTPError 保留上游 HTTP 状态，供业务层判断是否可切换备用模型。
type sub2APIHTTPError struct {
	statusCode int
	message    string
}

func (e *sub2APIHTTPError) Error() string {
	return fmt.Sprintf("Sub2API Responses 返回 HTTP %d: %s", e.statusCode, e.message)
}

// Retryable 返回错误是否属于限流或临时上游故障。
func (e *sub2APIHTTPError) Retryable() bool {
	return e.statusCode == http.StatusTooManyRequests || e.statusCode >= http.StatusInternalServerError
}

// NewSub2APIClient 创建固定模型的 Sub2API Responses 客户端。
// 输入：cfg 提供 BaseURL 和 APIKey，model 是本客户端使用的模型，httpClient 提供超时。
// 输出：返回可并发复用客户端。
// 副作用：无。
func NewSub2APIClient(cfg config.Sub2API, model string, httpClient *http.Client) *Sub2APIClient {
	// 1. 应用默认超时并规范基础地址和模型名。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 90 * time.Second}
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &Sub2APIClient{config: cfg, model: strings.TrimSpace(model), httpClient: httpClient}
}

// Configured 返回 Sub2API 地址、Key 和模型是否齐全。
// 输入：无。
// 输出：三个必要字段均非空时返回 true。
// 副作用：无。
func (c *Sub2APIClient) Configured() bool {
	// 1. 只返回配置状态，不暴露任何凭据。
	return c.config.BaseURL != "" && strings.TrimSpace(c.config.APIKey) != "" && c.model != ""
}

// SimpleChat 通过 OpenAI Responses API 发送单轮文本请求。
// 输入：ctx 控制请求，prompt 是文本输入，maxTokens 是输出上限。
// 输出：合并返回所有 output_text；配置、HTTP 或响应结构无效时返回错误。
// 副作用：调用 Sub2API 外部接口。
func (c *Sub2APIClient) SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// 1. 校验配置并序列化 Responses 请求。
	if !c.Configured() {
		return "", fmt.Errorf("Sub2API 配置不完整")
	}
	if maxTokens < 1 {
		maxTokens = 512
	}
	payload, err := json.Marshal(responsesRequest{
		Model: c.model, Input: prompt, MaxOutputTokens: maxTokens, Store: false,
	})
	if err != nil {
		return "", fmt.Errorf("序列化 Sub2API Responses 请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("创建 Sub2API Responses 请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.config.APIKey))
	request.Header.Set("Content-Type", "application/json")

	// 2. 执行请求并限制响应体大小。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("请求 Sub2API Responses: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("读取 Sub2API Responses 响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &sub2APIHTTPError{statusCode: response.StatusCode, message: responsesErrorMessage(body)}
	}

	// 3. 按官方 Responses 结构合并消息中的 output_text。
	var result responsesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("Sub2API Responses 返回非 JSON: %w", err)
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return "", fmt.Errorf("Sub2API Responses 返回错误: %s", result.Error.Message)
	}
	texts := make([]string, 0)
	for _, output := range result.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				texts = append(texts, content.Text)
			}
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("Sub2API Responses 没有返回 output_text，状态=%s", result.Status)
	}
	return strings.Join(texts, "\n"), nil
}

// responsesErrorMessage 从 Responses 错误响应提取可读说明。
// 输入：body 是上游错误响应体。
// 输出：优先返回 error.message，否则返回截断原文。
// 副作用：无。
func responsesErrorMessage(body []byte) string {
	// 1. 优先读取标准 Responses error.message。
	var result responsesResponse
	if json.Unmarshal(body, &result) == nil && result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return result.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	if text == "" {
		return "上游未返回错误说明"
	}
	return text
}
