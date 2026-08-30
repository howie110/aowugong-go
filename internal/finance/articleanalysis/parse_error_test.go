package articleanalysis

import (
	"errors"
	"strings"
	"testing"
)

// TestCombineArticleParseErrorsPreservesBothCauses 验证解析失败不会丢失详情接口的真实原因。
func TestCombineArticleParseErrorsPreservesBothCauses(t *testing.T) {
	detailErr := errors.New("微信读书要求人工验证")
	originalErr := errors.New("微信原文返回环境异常验证页")

	err := combineArticleParseErrors(detailErr, originalErr)
	if !errors.Is(err, detailErr) || !errors.Is(err, originalErr) {
		t.Fatalf("combineArticleParseErrors() = %v, want both causes", err)
	}
	if message := err.Error(); !strings.Contains(message, "微信读书详情") || !strings.Contains(message, "微信原文") {
		t.Fatalf("combineArticleParseErrors() = %q, want both contexts", message)
	}
}
