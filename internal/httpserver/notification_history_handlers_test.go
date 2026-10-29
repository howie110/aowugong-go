package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseNotificationLogFilter 验证通知历史筛选参数的默认值、解析和边界校验。
// 输入：合法和非法查询字符串。
// 输出：合法参数得到规范化筛选，非法参数返回错误。
// 副作用：无。
func TestParseNotificationLogFilter(t *testing.T) {
	// 1. 解析完整筛选条件。
	request := httptest.NewRequest("GET", "/?q=Vaultwarden&status=failed&category=监控&page=2&page_size=50", nil)
	filter, err := parseNotificationLogFilter(request)
	if err != nil {
		t.Fatalf("parseNotificationLogFilter() error = %v", err)
	}
	if filter.Query != "Vaultwarden" || filter.Status != "failed" || filter.Category != "监控" ||
		filter.Page != 2 || filter.PageSize != 50 {
		t.Fatalf("filter = %#v", filter)
	}

	// 2. 空参数使用默认分页。
	defaultFilter, err := parseNotificationLogFilter(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("default parse error = %v", err)
	}
	if defaultFilter.Page != 1 || defaultFilter.PageSize != 20 {
		t.Fatalf("default filter = %#v", defaultFilter)
	}

	// 3. 非法参数必须被拒绝。
	for _, test := range []struct {
		name string
		url  string
	}{
		{name: "页码为零", url: "/?page=0"},
		{name: "页码非数字", url: "/?page=abc"},
		{name: "每页为零", url: "/?page_size=0"},
		{name: "每页过大", url: "/?page_size=101"},
		{name: "状态非法", url: "/?status=bad"},
		{name: "分类非法", url: "/?category=bad"},
		{name: "搜索过长", url: "/?q=" + strings.Repeat("a", 101)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseNotificationLogFilter(httptest.NewRequest("GET", test.url, nil)); err == nil {
				t.Fatal("parseNotificationLogFilter() error = nil, want error")
			}
		})
	}
}
