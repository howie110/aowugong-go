package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ocr "github.com/alibabacloud-go/ocr-api-20210707/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/howiedata/aowugong-go/internal/config"
)

// AliyunOCRClient 使用阿里云官方 Go SDK 调用通用文字识别。
type AliyunOCRClient struct {
	client   *ocr.Client
	setupErr error
}

// NewAliyunOCRClient 创建阿里云通用文字识别客户端。
// 输入：cfg 包含端点和访问密钥。
// 输出：始终返回客户端；配置错误会延迟到实际识别时返回。
// 副作用：无，不发起网络请求。
func NewAliyunOCRClient(cfg config.PositionOCR) *AliyunOCRClient {
	// 1. 缺少凭据时保留可启动客户端，并记录上传时应返回的错误。
	if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.AccessKeySecret) == "" {
		return &AliyunOCRClient{setupErr: fmt.Errorf("必须配置 ALIYUN_OCR_ACCESS_KEY_ID 和 ALIYUN_OCR_ACCESS_KEY_SECRET")}
	}

	// 2. 使用官方 SDK 创建可复用客户端。
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "ocr-api.cn-hangzhou.aliyuncs.com"
	}
	sdk, err := ocr.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		Endpoint:        tea.String(endpoint),
		Protocol:        tea.String("HTTPS"),
	})
	if err != nil {
		return &AliyunOCRClient{setupErr: fmt.Errorf("创建阿里云 OCR 客户端: %w", err)}
	}
	return &AliyunOCRClient{client: sdk}
}

// Recognize 识别一张图片并返回项目统一 OCR 结构。
// 输入：ctx 控制调用前取消，image 是图片二进制。
// 输出：返回包含 data 和 request_id 的对象；配置或请求失败时返回错误。
// 副作用：调用阿里云 OCR 外部接口。
func (c *AliyunOCRClient) Recognize(ctx context.Context, image []byte) (map[string]any, error) {
	// 1. 在网络调用前处理取消和客户端配置错误。
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("调用阿里云 OCR: %w", err)
	}
	if c.setupErr != nil {
		return nil, c.setupErr
	}
	if len(image) == 0 {
		return nil, fmt.Errorf("调用阿里云 OCR: 图片内容为空")
	}

	// 2. 通过官方 SDK 发送二进制 Body。
	response, err := c.client.RecognizeGeneralWithOptions(
		&ocr.RecognizeGeneralRequest{Body: bytes.NewReader(image)},
		&util.RuntimeOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("调用阿里云 OCR: %w", err)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("调用阿里云 OCR: 响应为空")
	}

	// 3. 解码 SDK 返回的 Data JSON 字符串。
	result, err := normalizeAliyunOCRResponse(
		tea.StringValue(response.Body.Code),
		tea.StringValue(response.Body.Message),
		tea.StringValue(response.Body.RequestId),
		tea.StringValue(response.Body.Data),
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// normalizeAliyunOCRResponse 把 SDK 响应字段转换为项目统一结构。
// 输入：code、message、requestID 和 data 是阿里云响应字段。
// 输出：成功返回嵌套 data 对象；服务错误或无效 JSON 时返回错误。
// 副作用：无。
func normalizeAliyunOCRResponse(code, message, requestID, data string) (map[string]any, error) {
	// 1. 阿里云返回显式非成功状态时保留请求 ID 便于排查。
	if code != "" && code != "200" {
		return nil, fmt.Errorf("阿里云 OCR 失败 code=%s request_id=%s: %s", code, requestID, message)
	}

	// 2. Data 通常是 JSON 字符串，非 JSON 时兼容为纯 content。
	parsed := make(map[string]any)
	if strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			parsed = map[string]any{"content": data}
		}
	}
	return map[string]any{
		"code":       code,
		"message":    message,
		"request_id": requestID,
		"data":       parsed,
	}, nil
}
