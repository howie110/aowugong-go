package articleanalysis

import (
	"context"
	"fmt"
	"strings"
)

// AnalysisModelSettings 返回当前模型选择和完整模型目录。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回当前有效模型及各模型配置状态。
// 副作用：只读 PostgreSQL。
func (s *Service) AnalysisModelSettings(ctx context.Context) (AnalysisModelSettings, error) {
	// 1. 解析持久化选择；无设置或旧设置无效时使用当前默认模型。
	selected, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return AnalysisModelSettings{}, err
	}

	// 2. 按配置顺序输出目录，未配置 Key 的保留显示但不可选择。
	choices := make([]AnalysisModelChoice, 0, len(s.analysisModelOrder))
	for _, id := range s.analysisModelOrder {
		model := s.analysisModels[id]
		choices = append(choices, AnalysisModelChoice{
			ID: id, Provider: model.Provider, Model: model.Model, Label: model.Label,
			Configured: model.Analyzer.Configured(),
		})
	}
	return AnalysisModelSettings{
		SelectedModelID: selected.ID, SelectedModel: selected.Model,
		AnalysisPrompt: AnalysisPromptTemplate(), PromptVersion: PromptVersion,
		Models: choices,
	}, nil
}

// SetAnalysisModel 保存后续文章分析和信号归类使用的模型。
// 输入：ctx 控制写入，modelID 必须来自当前模型目录且已配置。
// 输出：返回更新后的模型设置；模型不可用时返回错误。
// 副作用：写入 PostgreSQL 模型设置。
func (s *Service) SetAnalysisModel(ctx context.Context, modelID string) (AnalysisModelSettings, error) {
	// 1. 拒绝目录外或缺少凭据的模型，避免页面保存一个必然失败的选项。
	model, exists := s.analysisModels[strings.TrimSpace(modelID)]
	if !exists {
		return AnalysisModelSettings{}, fmt.Errorf("文章分析模型不存在")
	}
	if !model.Analyzer.Configured() {
		return AnalysisModelSettings{}, fmt.Errorf("文章分析模型 %s 尚未配置凭据", model.Label)
	}
	if err := s.repository.SaveAnalysisModelID(ctx, model.ID); err != nil {
		return AnalysisModelSettings{}, err
	}
	return s.AnalysisModelSettings(ctx)
}

// selectedAnalysisModel 解析当前持久化选择并回退到第一个已配置模型。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回本次调用固定使用的模型运行时；没有模型目录时返回空运行时。
// 副作用：只读 PostgreSQL。
func (s *Service) selectedAnalysisModel(ctx context.Context) (analysisModelRuntime, error) {
	// 1. 单模型兼容模式无需额外查询，避免改变旧测试和简单调用行为。
	if len(s.analysisModelOrder) == 1 {
		return s.analysisModels[s.analysisModelOrder[0]], nil
	}
	selectedID := ""
	if len(s.analysisModelOrder) > 1 {
		var err error
		selectedID, err = s.repository.AnalysisModelID(ctx)
		if err != nil {
			return analysisModelRuntime{}, err
		}
	}
	if selected, exists := s.analysisModels[selectedID]; exists && selected.Analyzer.Configured() {
		return selected, nil
	}
	if fallback, exists := s.analysisModels[s.defaultAnalysisModelID]; exists && fallback.Analyzer.Configured() {
		return fallback, nil
	}
	for _, id := range s.analysisModelOrder {
		if candidate := s.analysisModels[id]; candidate.Analyzer.Configured() {
			return candidate, nil
		}
	}
	if fallback, exists := s.analysisModels[s.defaultAnalysisModelID]; exists {
		return fallback, nil
	}
	if len(s.analysisModelOrder) > 0 {
		return s.analysisModels[s.analysisModelOrder[0]], nil
	}
	return analysisModelRuntime{}, nil
}
