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
)

const monitorResponseLimit = 4 << 20

// ProbeResult 描述一次外部 HTTP 探测结果。
type ProbeResult struct {
	Healthy    bool
	HTTPStatus *int
	LatencyMS  int
	Message    string
}

// MonitoringClient 封装服务监控使用的外部 HTTP 探测。
type MonitoringClient struct {
	http *http.Client
}

// NewMonitoringClient 创建服务监控客户端。
// 输入：httpClient 是应用共享的 HTTP 客户端。
// 输出：返回监控客户端。
// 副作用：无。
func NewMonitoringClient(httpClient *http.Client) *MonitoringClient {
	// 1. 保存显式注入的 HTTP 客户端。
	return &MonitoringClient{http: httpClient}
}

// ProbeURL 使用 GET 判断普通服务是否可访问。
// 输入：ctx 是调用上下文，address 是目标 URL。
// 输出：HTTP 5xx 或网络错误标记为不健康。
// 副作用：调用外部 HTTP 服务。
func (c *MonitoringClient) ProbeURL(ctx context.Context, address string) ProbeResult {
	// 1. 构造带监控 User-Agent 的 GET 请求。
	startedAt := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	request.Header.Set("User-Agent", "aowugong-service-monitor/1.0")

	// 2. 执行请求并仅按 5xx 判定普通服务不可用。
	response, err := c.http.Do(request)
	latency := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		return ProbeResult{LatencyMS: latency, Message: truncateText(err.Error(), 1000)}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	status := response.StatusCode
	if status >= 500 {
		return ProbeResult{HTTPStatus: &status, LatencyMS: latency, Message: fmt.Sprintf("HTTP %d", status)}
	}
	return ProbeResult{Healthy: true, HTTPStatus: &status, LatencyMS: latency}
}

// ProbeOpenILink 使用空内容请求静默验证发送接口，不投递真实消息。
// 输入：ctx 是调用上下文，address 是发送接口，appToken 是 Bridge App Token。
// 输出：只有预期的 content is required 响应视为健康。
// 副作用：调用 OpeniLink 外部 HTTP API，但不发送有效消息。
func (c *MonitoringClient) ProbeOpenILink(ctx context.Context, address, appToken string) ProbeResult {
	// 1. 构造空文本探测请求。
	startedAt := time.Now()
	body := []byte(`{"type":"text","content":""}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	request.Header.Set("Authorization", "Bearer "+appToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "aowugong-service-monitor/1.0")

	// 2. 执行请求并解析有限 JSON 错误载荷。
	response, err := c.http.Do(request)
	latency := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		return ProbeResult{LatencyMS: latency, Message: "OpeniLink 静默发送探测失败：" + truncateText(err.Error(), 900)}
	}
	defer response.Body.Close()
	status := response.StatusCode
	content, _ := io.ReadAll(io.LimitReader(response.Body, monitorResponseLimit))
	var payload map[string]any
	if json.Unmarshal(content, &payload) != nil {
		return ProbeResult{HTTPStatus: &status, LatencyMS: latency, Message: fmt.Sprintf("OpeniLink 静默发送探测返回非 JSON：HTTP %d", status)}
	}
	message := StringValue(payload["error"])
	if message == "" {
		message = StringValue(payload["message"])
	}
	if status == http.StatusBadRequest && strings.Contains(message, "content is required") {
		healthyStatus := http.StatusOK
		return ProbeResult{Healthy: true, HTTPStatus: &healthyStatus, LatencyMS: latency}
	}
	if status >= 400 {
		return ProbeResult{HTTPStatus: &status, LatencyMS: latency, Message: truncateText(fmt.Sprintf("OpeniLink 静默发送探测失败：HTTP %d %s", status, message), 1000)}
	}
	return ProbeResult{HTTPStatus: &status, LatencyMS: latency, Message: "OpeniLink 静默发送探测意外成功；为避免发送测试消息，判定为异常"}
}

// truncateText 按字符数截断外部错误文本。
// 输入：value 是文本，limit 是最大字符数。
// 输出：返回截断结果。
// 副作用：无。
func truncateText(value string, limit int) string {
	// 1. 使用 rune 避免破坏 UTF-8。
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
