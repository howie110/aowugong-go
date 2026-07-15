package articleanalysis

import "testing"

// TestNormalizeAnalysisMapsCautiousAndResolvesSignalConflict 验证市场枚举和单标的单结论规则。
// 输入：cautious 市场氛围和同时出现在推荐、风险中的同一标的。
// 输出：氛围变为 neutral，标的只保留风险侧最终结论。
// 副作用：无。
func TestNormalizeAnalysisMapsCautiousAndResolvesSignalConflict(t *testing.T) {
	// 1. 构造模型可能返回的冲突结构。
	input := AnalysisResult{
		Summary:         "测试摘要",
		Market:          MarketJudgment{Mood: "cautious", Prediction: "up"},
		Recommendations: []Signal{{Name: "某公司", Type: "stock", Reason: "可能改善"}},
		Risks:           []Signal{{Name: "某公司", Type: "stock", Reason: "盈利不及预期且下行压力明显"}},
	}

	// 2. 规范化并核对唯一最终方向。
	result := NormalizeAnalysis(input)
	if result.Market.Mood != "neutral" {
		t.Errorf("mood = %q, want neutral", result.Market.Mood)
	}
	if result.Market.Prediction != "up" {
		t.Errorf("prediction = %q, want up", result.Market.Prediction)
	}
	if len(result.Recommendations) != 0 || len(result.Risks) != 1 || result.Risks[0].Name != "某公司" {
		t.Errorf("signals = recommendations %#v risks %#v", result.Recommendations, result.Risks)
	}
}
