package httpserver

import (
	"encoding/json"
	"net/http"
)

type errorEnvelope struct {
	Detail string    `json:"detail"`
	Error  errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON 写入统一格式的 JSON 响应。
// 输入：w 是响应器，status 是状态码，value 是可编码响应体。
// 输出：无；编码错误不会二次写响应。
// 副作用：设置响应头、状态码并写入 JSON。
func writeJSON(w http.ResponseWriter, status int, value any) {
	// 1. 设置响应状态与内容类型。
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// 2. 编码响应内容。
	_ = json.NewEncoder(w).Encode(value)
}

// writeError 写入统一 JSON 错误信封。
// 输入：w 是响应器，status、code 和 message 描述公开错误。
// 输出：无。
// 副作用：写入统一 JSON 错误响应。
func writeError(w http.ResponseWriter, status int, code, message string) {
	// 1. 使用标准响应编码器返回错误信息。
	writeJSON(w, status, errorEnvelope{
		Detail: message,
		Error:  errorBody{Code: code, Message: message},
	})
}
