package work

import (
	"os"
	"path/filepath"
	"testing"
)

// TestServiceNavigationNormalizesPrivateFile 验证私有导航文件会清洗链接并统计总数。
// 输入：包含分组、重复空白和链接的临时导航 JSON。
// 输出：返回规范化分组、入口和统计数量。
// 副作用：创建并读取临时 JSON 文件。
func TestServiceNavigationNormalizesPrivateFile(t *testing.T) {
	// 1. 写入包含有效、旧字段和无效链接的私有 JSON。
	path := filepath.Join(t.TempDir(), "navigation.json")
	content := `{
		"title":" 我的导航 ",
		"groups":[{"title":" 常用 ","links":[
			{"title":"站点","url":"https://example.com/path"},
			{"web_title":"旧链接","web_url":"intranet.local"},
			{"title":"缺地址"}
		]}]
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 读取后应保留两个有效链接及展示 host。
	navigation, err := NewService(path).Navigation()
	if err != nil {
		t.Fatalf("Navigation() error = %v", err)
	}
	if navigation.Title != "我的导航" || navigation.Total != 2 || !navigation.IsConfigured {
		t.Errorf("navigation = %#v", navigation)
	}
	if navigation.Groups[0].Links[0].Host != "example.com" || navigation.Groups[0].Links[1].Host != "intranet.local" {
		t.Errorf("hosts = %#v", navigation.Groups[0].Links)
	}
}

// TestServiceNavigationReturnsEmptyWhenMissing 验证私有导航未配置时返回稳定空状态。
// 输入：不存在的导航文件路径。
// 输出：返回空分组和零统计而不报错。
// 副作用：只检查临时路径元数据。
func TestServiceNavigationReturnsEmptyWhenMissing(t *testing.T) {
	// 1. 使用不存在的文件路径读取导航。
	navigation, err := NewService(filepath.Join(t.TempDir(), "missing.json")).Navigation()
	if err != nil {
		t.Fatalf("Navigation() error = %v", err)
	}

	// 2. 响应必须可直接供页面渲染。
	if navigation.IsConfigured || navigation.Total != 0 || len(navigation.Groups) != 0 {
		t.Errorf("navigation = %#v, want empty", navigation)
	}
}
