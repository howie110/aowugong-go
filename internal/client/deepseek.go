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

// DeepSeekClient 调用兼容 OpenAI 协议的 DeepSeek Chat Completions。
type DeepSeekClient struct {
	config     config.DeepSeek
	httpClient *http.Client
}

type chatRequest struct {
	Model       string            `json:"model"`
	Messages    []chatMessage     `json:"messages"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	Stream      bool              `json:"stream"`
	Thinking    map[string]string `json:"thinking,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error map[string]any `json:"error"`
}

// NewDeepSeekClient 创建 DeepSeek 客户端。
// 输入：cfg 提供 BaseURL、APIKey 和模型，httpClient 提供超时与连接复用。
// 输出：返回可并发复用客户端。
// 副作用：无。
func NewDeepSeekClient(cfg config.DeepSeek, httpClient *http.Client) *DeepSeekClient {
	// 1. 应用默认超时并清理基础地址。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &DeepSeekClient{config: cfg, httpClient: httpClient}
}

// Configured 返回 DeepSeek API Key 是否已配置。
// 输入：无。
// 输出：密钥非空时返回 true。
// 副作用：无。
func (c *DeepSeekClient) Configured() bool {
	// 1. 不暴露密钥，只返回状态。
	return strings.TrimSpace(c.config.APIKey) != ""
}

// SimpleChat 发送单轮提示词并返回第一条助手文本。
// 输入：ctx 控制请求，prompt 是用户消息，maxTokens 是输出上限。
// 输出：返回 assistant content；配置或响应无效时返回错误。
// 副作用：调用 DeepSeek 外部接口。
func (c *DeepSeekClient) SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// 1. 校验配置并序列化 OpenAI 兼容请求。
	if !c.Configured() {
		return "", fmt.Errorf("未配置 DEEPSEEK_API_KEY")
	}
	if maxTokens < 1 {
		maxTokens = 512
	}
	payload, err := json.Marshal(chatRequest{
		Model: c.config.Model, Messages: []chatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.2, MaxTokens: maxTokens, Stream: false,
		Thinking: map[string]string{"type": "disabled"},
	})
	if err != nil {
		return "", fmt.Errorf("序列化 DeepSeek 请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("创建 DeepSeek 请求: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	request.Header.Set("Content-Type", "application/json")

	// 2. 执行请求并限制响应体大小。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("请求 DeepSeek: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("读取 DeepSeek 响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("DeepSeek 接口返回 HTTP %d: %s", response.StatusCode, deepSeekErrorMessage(body))
	}

	// 3. 解码并返回第一条非空助手文本。
	var result chatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("DeepSeek 接口返回非 JSON: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("DeepSeek 接口没有返回 choices")
	}
	return result.Choices[0].Message.Content, nil
}

// deepSeekErrorMessage 从错误响应提取可读信息。
// 输入：body 是 DeepSeek 错误响应体。
// 输出：返回 error.message 或截断后的原文。
// 副作用：无。
func deepSeekErrorMessage(body []byte) string {
	// 1. 优先解析标准 error.message。
	var result chatResponse
	if json.Unmarshal(body, &result) == nil {
		if message, ok := result.Error["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
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
