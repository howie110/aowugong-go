package client

import "testing"

// TestNormalizeAliyunOCRResponseDecodesDataJSON 验证阿里云 OCR Data 字符串规范化。
// 输入：包含 content 和坐标数组的 Data JSON 文本。
// 输出：返回 position 解析器使用的 data 对象和请求 ID。
// 副作用：无。
func TestNormalizeAliyunOCRResponseDecodesDataJSON(t *testing.T) {
	// 1. 规范化代表性 SDK 响应字段。
	result, err := normalizeAliyunOCRResponse("200", "", "request-1", `{"content":"总资产 100.00","prism_wordsInfo":[]}`)
	if err != nil {
		t.Fatalf("normalizeAliyunOCRResponse() error = %v", err)
	}

	// 2. 核对统一字段和嵌套内容。
	if result["request_id"] != "request-1" {
		t.Errorf("request_id = %#v, want request-1", result["request_id"])
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["content"] != "总资产 100.00" {
		t.Errorf("data = %#v, want decoded content", result["data"])
	}
}
