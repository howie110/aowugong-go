package articleanalysis

import (
	"context"
	"errors"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type recordingAnalysisGateway struct {
	content string
	err     error
	calls   int
}

func (g *recordingAnalysisGateway) Configured() bool { return true }

func (g *recordingAnalysisGateway) SimpleChat(context.Context, string, int) (string, error) {
	g.calls++
	return g.content, g.err
}

type retryableAnalysisTestError struct{}

func (retryableAnalysisTestError) Error() string   { return "temporary upstream failure" }
func (retryableAnalysisTestError) Retryable() bool { return true }

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

// TestServiceFallsBackToDeepSeekForRetryableSelectedModelError 验证临时模型故障不会拖垮整批文章。
func TestServiceFallsBackToDeepSeekForRetryableSelectedModelError(t *testing.T) {
	primary := &recordingAnalysisGateway{err: retryableAnalysisTestError{}}
	fallback := &recordingAnalysisGateway{content: "fallback result"}
	service := NewService(nil, ServiceOptions{
		AnalysisModels: []AnalysisModelConfig{
			{ID: "sub2api:gpt-5.6-luna", Provider: "sub2api", Model: "gpt-5.6-luna", Analyzer: primary},
			{ID: "deepseek:deepseek-v4-pro", Provider: "deepseek", Model: "deepseek-v4-pro", Analyzer: fallback},
		},
		DefaultAnalysisModelID: "sub2api:gpt-5.6-luna",
	})

	content, used, err := service.callAnalysisModel(context.Background(), service.analysisModels["sub2api:gpt-5.6-luna"], "prompt", 100)
	if err != nil || content != "fallback result" || used.ID != "deepseek:deepseek-v4-pro" {
		t.Fatalf("callAnalysisModel() = %q, %#v, %v", content, used, err)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls = primary %d, fallback %d", primary.calls, fallback.calls)
	}
}

// TestServiceDoesNotFallbackForPermanentModelError 验证配置或请求错误仍原样暴露。
func TestServiceDoesNotFallbackForPermanentModelError(t *testing.T) {
	primaryErr := errors.New("invalid request")
	primary := &recordingAnalysisGateway{err: primaryErr}
	fallback := &recordingAnalysisGateway{content: "must not run"}
	service := NewService(nil, ServiceOptions{
		AnalysisModels: []AnalysisModelConfig{
			{ID: "sub2api:gpt-5.6-luna", Provider: "sub2api", Model: "gpt-5.6-luna", Analyzer: primary},
			{ID: "deepseek:deepseek-v4-pro", Provider: "deepseek", Model: "deepseek-v4-pro", Analyzer: fallback},
		},
	})

	_, _, err := service.callAnalysisModel(context.Background(), service.analysisModels["sub2api:gpt-5.6-luna"], "prompt", 100)
	if !errors.Is(err, primaryErr) || fallback.calls != 0 {
		t.Fatalf("callAnalysisModel() error = %v, fallback calls = %d", err, fallback.calls)
	}
}
