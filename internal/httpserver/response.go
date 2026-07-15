package httpserver

import (
	"encoding/json"
	"net/http"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON 写入统一格式的 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, value any) {
	// 1. 设置响应状态与内容类型。
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// 2. 编码响应内容。
	_ = json.NewEncoder(w).Encode(value)
}

// writeError 写入统一 JSON 错误信封。
func writeError(w http.ResponseWriter, status int, code, message string) {
	// 1. 使用标准响应编码器返回错误信息。
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}
