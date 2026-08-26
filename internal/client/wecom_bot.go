package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/howiedata/aowugong-go/internal/config"
)

const (
	weComBotResponseLimit = 1 << 20
	weComTextLimit        = 2048
)

// WeComBotClient 通过企业微信群机器人发送个人微信可显示的纯文本消息。
type WeComBotClient struct {
	config config.WeComBot
	http   *http.Client
}

// NewWeComBotClient 创建企业微信群机器人客户端。
func NewWeComBotClient(cfg config.WeComBot, httpClient *http.Client) *WeComBotClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &WeComBotClient{config: cfg, http: httpClient}
}

// Configured 返回官方 Webhook 地址是否已经配置。
func (c *WeComBotClient) Configured() bool {
	return validateWeComBotWebhook(c.config.WebhookURL) == nil
}

// SendText 发送企业微信群机器人 text 消息。
func (c *WeComBotClient) SendText(ctx context.Context, content string) error {
	address := strings.TrimSpace(c.config.WebhookURL)
	if err := validateWeComBotWebhook(address); err != nil {
		return err
	}
	content = truncateUTF8Bytes(strings.TrimSpace(content), weComTextLimit)
	if content == "" {
		return fmt.Errorf("企业微信通知正文不能为空")
	}

	payload := struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}{MsgType: "text"}
	payload.Text.Content = content
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化企业微信通知: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建企业微信通知请求: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "aowugong-wecom-bot/1.0")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("请求企业微信群机器人: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, weComBotResponseLimit))
	if err != nil {
		return fmt.Errorf("读取企业微信响应: %w", err)
	}
	var result struct {
		ErrorCode int    `json:"errcode"`
		Message   string `json:"errmsg"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("企业微信接口返回非 JSON: HTTP %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("企业微信接口返回 HTTP %d: %s", response.StatusCode, result.Message)
	}
	if result.ErrorCode != 0 {
		return fmt.Errorf("企业微信接口返回错误 %d: %s", result.ErrorCode, result.Message)
	}
	return nil
}

func validateWeComBotWebhook(address string) error {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "qyapi.weixin.qq.com" || parsed.Port() != "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Path != "/cgi-bin/webhook/send" {
		return fmt.Errorf("WECOM_BOT_WEBHOOK_URL 必须是企业微信官方 HTTPS Webhook")
	}
	query := parsed.Query()
	if len(query) != 1 || len(query["key"]) != 1 || strings.TrimSpace(query.Get("key")) == "" {
		return fmt.Errorf("WECOM_BOT_WEBHOOK_URL 缺少有效 key")
	}
	return nil
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	suffix := "\n[内容已截断]"
	limit -= len(suffix)
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value + suffix
}
