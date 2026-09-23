package vpn

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// parseIntDefault 把可选十进制文本转换为整数。
// 输入：value 是十进制文本，fallback 是空值或无效值回退。
// 输出：返回解析整数或 fallback。
// 副作用：无。
func parseIntDefault(value string, fallback int) int {
	// 1. 解析失败时保持调用方指定默认值。
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// valueString 把 YAML 动态标量转换为稳定字符串。
// 输入：value 是 YAML 或 JSON 标量。
// 输出：返回无科学计数法的文本；空值返回空字符串。
// 副作用：无。
func valueString(value any) string {
	// 1. 覆盖 YAML 常见标量类型并对其他值使用 fmt。
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// valueBool 把 YAML 动态标量转换为布尔值。
// 输入：value 是 YAML 或 JSON 标量。
// 输出：布尔真或文本 true 时返回 true。
// 副作用：无。
func valueBool(value any) bool {
	// 1. 接受解析器产生的布尔值及常见文本形式。
	if typed, ok := value.(bool); ok {
		return typed
	}
	parsed, _ := strconv.ParseBool(valueString(value))
	return parsed
}

// querySuffix 把非空查询参数编码为 URL 后缀。
// 输入：query 是查询参数。
// 输出：空参数返回空字符串，否则返回问号开头的编码值。
// 副作用：无。
func querySuffix(query url.Values) string {
	// 1. 只在存在参数时添加问号。
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

// firstNonEmpty 返回第一个非空字符串。
// 输入：values 是候选文本。
// 输出：返回清理后的第一个非空值。
// 副作用：无。
func firstNonEmpty(values ...string) string {
	// 1. 保持候选优先级并跳过空白。
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

// mapStringAny 把字符串映射转换为 YAML 动态映射。
// 输入：values 是字符串键值。
// 输出：返回相同数据的 any 映射。
// 副作用：无。
func mapStringAny(values map[string]string) map[string]any {
	// 1. 逐项复制，避免调用方修改原始映射。
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
