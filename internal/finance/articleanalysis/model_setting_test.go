package articleanalysis

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServicePersistsArticleAnalysisModelSelection 验证页面选择会被后续任务读取。
// 输入：两个已配置的测试模型和空设置数据库。
// 输出：默认选择第一个，保存后稳定选择第二个。
// 副作用：创建并写入隔离 SQLite 数据库。
func TestServicePersistsArticleAnalysisModelSelection(t *testing.T) {
	// 1. 构造包含 Sub2API 和 DeepSeek 两个选项的服务。
	ctx := context.Background()
	repository := NewRepository(testdatabase.Open(t))
	service := NewService(repository, ServiceOptions{
		AnalysisModels: []AnalysisModelConfig{
			{ID: "sub2api:gpt-5.6-luna", Provider: "sub2api", Model: "gpt-5.6-luna", Label: "gpt-5.6-luna", Analyzer: fixedAnalysisGateway{}},
			{ID: "deepseek:deepseek-v4-pro", Provider: "deepseek", Model: "deepseek-v4-pro", Label: "deepseek-v4-pro", Analyzer: fixedAnalysisGateway{}},
		},
		DefaultAnalysisModelID: "sub2api:gpt-5.6-luna",
	})
	settings, err := service.AnalysisModelSettings(ctx)
	if err != nil || settings.SelectedModelID != "sub2api:gpt-5.6-luna" || len(settings.Models) != 2 {
		t.Fatalf("initial settings = %#v, %v", settings, err)
	}

	// 2. 保存 DeepSeek 并通过新的读取确认数据库选择生效。
	settings, err = service.SetAnalysisModel(ctx, "deepseek:deepseek-v4-pro")
	if err != nil || settings.SelectedModelID != "deepseek:deepseek-v4-pro" || settings.SelectedModel != "deepseek-v4-pro" {
		t.Fatalf("updated settings = %#v, %v", settings, err)
	}
	selected, err := service.selectedAnalysisModel(ctx)
	if err != nil || selected.Model != "deepseek-v4-pro" {
		t.Fatalf("selected model = %#v, %v", selected, err)
	}
}
