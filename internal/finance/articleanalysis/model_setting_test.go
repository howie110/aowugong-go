package articleanalysis

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServicePersistsArticleAnalysisModelSelection 验证页面选择会被后续任务读取。
// 输入：一个已配置的 DeepSeek 测试模型和空设置数据库。
// 输出：默认选择 DeepSeek，模型目录只有一个选项。
// 副作用：创建并写入隔离 SQLite 数据库。
func TestServicePersistsArticleAnalysisModelSelection(t *testing.T) {
	// 1. 构造只包含 DeepSeek 的服务。
	ctx := context.Background()
	repository := NewRepository(testdatabase.Open(t))
	service := NewService(repository, ServiceOptions{
		AnalysisModels: []AnalysisModelConfig{
			{ID: "deepseek:deepseek-v4-pro", Provider: "deepseek", Model: "deepseek-v4-pro", Label: "deepseek-v4-pro", Analyzer: fixedAnalysisGateway{}},
		},
		DefaultAnalysisModelID: "deepseek:deepseek-v4-pro",
	})
	settings, err := service.AnalysisModelSettings(ctx)
	if err != nil || settings.SelectedModelID != "deepseek:deepseek-v4-pro" || len(settings.Models) != 1 {
		t.Fatalf("initial settings = %#v, %v", settings, err)
	}
	if settings.PromptVersion != PromptVersion || settings.AnalysisPrompt != AnalysisPromptTemplate() {
		t.Fatalf("prompt settings = %#v", settings)
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
