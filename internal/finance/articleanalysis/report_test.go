package articleanalysis

import (
	"strings"
	"testing"
)

// TestBuildDistributionPreservesFirstSeenOrderForEqualCounts 验证同次数市场判断沿用旧接口的首次出现顺序。
// 输入：依次出现且次数相同的 unknown 和 down 预测。
// 输出：unknown 保持在 down 之前。
// 副作用：无。
func TestBuildDistributionPreservesFirstSeenOrderForEqualCounts(t *testing.T) {
	// 1. 以仓储查询的倒序结果模拟旧接口字典首次插入顺序。
	rows := []analysisRow{
		{Prediction: "unknown"},
		{Prediction: "down"},
	}

	// 2. 同次数时不能改成名称字母序。
	result := buildDistribution(rows, false)
	if len(result) != 2 || result[0].Name != "unknown" || result[1].Name != "down" {
		t.Fatalf("distribution = %#v, want unknown before down", result)
	}
}

// TestBuildSignalStatsUsesRecommendationCountAsStableTieBreak 验证同总数、同日期信号不会随 map 顺序跳位。
// 输入：总次数相同但推荐次数不同的两个信号。
// 输出：推荐次数更多的信号稳定排在前面。
// 副作用：无。
func TestBuildSignalStatsUsesRecommendationCountAsStableTieBreak(t *testing.T) {
	// 1. 构造总数均为三次且最近日期相同的推荐、风险组合。
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "信号B"}}, Risks: []Signal{{Name: "信号A"}}, OccurredAt: "2026-07-15"},
		{Recommendations: []Signal{{Name: "信号B"}}, Risks: []Signal{{Name: "信号A"}}, OccurredAt: "2026-07-15"},
		{Recommendations: []Signal{{Name: "信号A"}}, Risks: []Signal{{Name: "信号B"}}, OccurredAt: "2026-07-15"},
	}

	// 2. 重复构建以覆盖 Go map 的随机遍历顺序。
	for attempt := 0; attempt < 64; attempt++ {
		result := buildSignalStats(rows)
		if len(result) != 2 || result[0].Name != "信号B" {
			t.Fatalf("attempt %d signals = %#v, want 信号B first", attempt, result)
		}
	}
}

// TestBuildSignalStatsAggregatesNameAndTypeBeforeFinalMerge 验证旧接口先按名称和类型聚合再合并名称。
// 输入：较新的一次 sector 风险和较高频的 concept 风险使用同一名称。
// 输出：最终类型采用聚合次数更多且先进入名称榜的 concept。
// 副作用：无。
func TestBuildSignalStatsAggregatesNameAndTypeBeforeFinalMerge(t *testing.T) {
	// 1. 按仓储的时间倒序构造同名不同类型信号。
	rows := []analysisRow{
		{Risks: []Signal{{Name: "高位科技股", Type: "sector"}}, OccurredAt: "2026-07-15"},
		{Risks: []Signal{{Name: "高位科技股", Type: "concept"}}, OccurredAt: "2026-07-14"},
		{Risks: []Signal{{Name: "高位科技股", Type: "concept"}}, OccurredAt: "2026-07-13"},
	}

	// 2. 较高频类型应在最终按名称合并时成为展示类型。
	result := buildSignalStats(rows)
	if len(result) != 1 || result[0].Type != "concept" || result[0].RiskCount != 3 {
		t.Fatalf("signals = %#v, want concept with three risks", result)
	}
}

// TestAnalysisPromptTemplateKeepsFinalV7Rules 验证 Go 分析提示词保留现有 v7 最终规则。
// 输入：页面展示使用的占位符提示词。
// 输出：结果导向示例、未来理由范围和推荐风险原因约束均完整存在。
// 副作用：无。
func TestAnalysisPromptTemplateKeepsFinalV7Rules(t *testing.T) {
	// 1. 读取页面与实际分析共用的唯一提示词模板。
	prompt := AnalysisPromptTemplate()

	// 2. 核对会直接影响模型最终效果的完整规则和 JSON 字段说明。
	required := []string{
		"例如“科技大涨虹吸传统行业”“年初至今盈利30-40%”“涨幅领先”“一枝独秀”等已经发生的涨跌、排名、收益结果",
		"例如估值、盈利/业绩、政策、供需、周期、库存、订单、流动性、风险事件、配置价值、催化或基本面变化",
		`"reason": "80字以内综合原因，说明为什么最终偏推荐"`,
		`"reason": "80字以内综合原因，说明为什么最终偏风险"`,
		`"mood_reason": "80字以内原因，说明文章为什么体现这种短期市场氛围"`,
		`"prediction_reason": "80字以内原因，说明文章为什么体现这种短期涨跌预测"`,
	}
	for _, fragment := range required {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt is missing %q", fragment)
		}
	}
	if strings.Contains(prompt, `"risks":[]`) {
		t.Error("prompt still uses the shortened risks example")
	}
}
