package articleanalysis

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

var fencedJSONPattern = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// parseAnalysisJSON 解码模型纯 JSON 或 Markdown fenced JSON，并容忍常见轻微格式噪声。
// 输入：content 是模型文本。
// 输出：返回结构化结果；JSON 无效时返回错误。
// 副作用：无。
func parseAnalysisJSON(content string) (AnalysisResult, error) {
	var result AnalysisResult
	if err := unmarshalJSONWithRepair(content, &result); err != nil {
		return AnalysisResult{}, err
	}
	return result, nil
}

// unmarshalJSONWithRepair 先严格解码，失败后只修复可证明的格式噪声。
// 输入：content 是模型响应，target 是目标结构。
// 输出：严格或安全修复后解码成功返回 nil，否则返回首次 JSON 错误。
// 副作用：无。
func unmarshalJSONWithRepair(content string, target any) error {
	content = extractJSONPayload(content)
	if err := json.Unmarshal([]byte(content), target); err == nil {
		return nil
	} else {
		repaired := repairJSONPayload(content)
		if repaired != content {
			if repairErr := json.Unmarshal([]byte(repaired), target); repairErr == nil {
				return nil
			}
		}
		return err
	}
}

// extractJSONPayload 提取 fenced JSON 或响应中第一个完整对象，去掉模型附带的说明和多余尾部。
// 输入：content 是模型原始文本。
// 输出：返回尽量不改变正文的 JSON 对象文本。
// 副作用：无。
func extractJSONPayload(content string) string {
	content = strings.TrimSpace(content)
	if matches := fencedJSONPattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}
	start := strings.IndexByte(content, '{')
	if start < 0 {
		return content
	}
	if end, ok := completeJSONObjectEnd(content[start:]); ok {
		return strings.TrimSpace(content[start : start+end])
	}
	return content
}

// completeJSONObjectEnd 返回从开头对象到最外层闭合符的下一个索引。
// 输入：value 必须从对象开头开始。
// 输出：完整对象的结束索引和是否完整。
// 副作用：无。
func completeJSONObjectEnd(value string) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for index, character := range value {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

// repairJSONPayload 删除对象或数组闭合前的尾逗号，避免把普通模型笔误当成整篇失败。
// 输入：content 是已经提取的 JSON 对象文本。
// 输出：返回修复后的文本；字符串内部的逗号保持不变。
// 副作用：无。
func repairJSONPayload(content string) string {
	var builder strings.Builder
	builder.Grow(len(content))
	inString := false
	escaped := false
	for index := 0; index < len(content); index++ {
		character := content[index]
		if inString {
			builder.WriteByte(character)
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			builder.WriteByte(character)
			continue
		}
		if character == ',' {
			next := index + 1
			for next < len(content) && unicode.IsSpace(rune(content[next])) {
				next++
			}
			if next < len(content) && (content[next] == '}' || content[next] == ']') {
				continue
			}
		}
		builder.WriteByte(character)
	}
	return builder.String()
}
