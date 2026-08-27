package articleanalysis

import (
	"strings"
	"testing"
)

// TestBuildDistributionReturnsEveryPredictionCategory 验证市场判断始终返回完整固定分类。
// 输入：只有 unknown 和 down 预测。
// 输出：up、range、down、unknown 按固定顺序返回，缺少类别计数为零。
// 副作用：无。
func TestBuildDistributionReturnsEveryPredictionCategory(t *testing.T) {
	rows := []analysisRow{
		{Prediction: "unknown"},
		{Prediction: "down"},
	}

	result := buildDistribution(rows, false)
	want := []DistributionItem{{Name: "up", Count: 0}, {Name: "range", Count: 0}, {Name: "down", Count: 1}, {Name: "unknown", Count: 1}}
	if len(result) != len(want) {
		t.Fatalf("distribution = %#v, want %#v", result, want)
	}
	for index := range want {
		if result[index] != want[index] {
			t.Fatalf("distribution[%d] = %#v, want %#v", index, result[index], want[index])
		}
	}
}

// TestBuildSignalStatsUsesRecommendationCountAsStableTieBreak 验证同总数、同日期信号不会随 map 顺序跳位。
// 输入：总次数相同但推荐次数不同的两个信号。
// 输出：推荐次数更多的信号稳定排在前面。
// 副作用：无。
func TestBuildSignalStatsUsesRecommendationCountAsStableTieBreak(t *testing.T) {
	// 1. 构造明确映射及总数均为三次、最近日期相同的推荐风险组合。
	groups := []SignalGroup{
		{ID: 1, Name: "信号A", Type: "other", Aliases: []string{"信号A"}},
		{ID: 2, Name: "信号B", Type: "other", Aliases: []string{"信号B"}},
	}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "信号B"}}, Risks: []Signal{{Name: "信号A"}}, OccurredAt: "2026-07-15"},
		{Recommendations: []Signal{{Name: "信号B"}}, Risks: []Signal{{Name: "信号A"}}, OccurredAt: "2026-07-15"},
		{Recommendations: []Signal{{Name: "信号A"}}, Risks: []Signal{{Name: "信号B"}}, OccurredAt: "2026-07-15"},
	}

	// 2. 重复构建以覆盖 Go map 的随机遍历顺序。
	for attempt := 0; attempt < 64; attempt++ {
		result := buildSignalStats(rows, groups)
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
	// 1. 按仓储的时间倒序构造同名不同类型信号，并给出最终概念映射。
	groups := []SignalGroup{{ID: 1, Name: "高位科技股", Type: "concept", Aliases: []string{"高位科技股"}}}
	rows := []analysisRow{
		{Risks: []Signal{{Name: "高位科技股", Type: "sector"}}, OccurredAt: "2026-07-15"},
		{Risks: []Signal{{Name: "高位科技股", Type: "concept"}}, OccurredAt: "2026-07-14"},
		{Risks: []Signal{{Name: "高位科技股", Type: "concept"}}, OccurredAt: "2026-07-13"},
	}

	// 2. 较高频类型应在最终按名称合并时成为展示类型。
	result := buildSignalStats(rows, groups)
	if len(result) != 1 || result[0].Type != "concept" || result[0].RiskCount != 3 {
		t.Fatalf("signals = %#v, want concept with three risks", result)
	}
}

// TestBuildSignalStatsGroupsAliasesAndCountsEveryOccurrence 验证概念组聚合别名且不对同篇文章去重。
// 输入：同一文章包含“券商”和“中信证券”，另一文章包含“券商板块”。
// 输出：三条原始信号合并为“证券行业”，完整返回三个成员并累计三次。
// 副作用：无。
func TestBuildSignalStatsGroupsAliasesAndCountsEveryOccurrence(t *testing.T) {
	// 1. 构造一个概念组和同篇文章内的两次组内命中。
	groups := []SignalGroup{{
		ID: 1, Name: "证券行业", Type: "sector",
		Aliases: []string{"券商", "券商板块", "中信证券"},
	}}
	rows := []analysisRow{
		{
			Recommendations: []Signal{{Name: "券商", Type: "sector"}, {Name: "中信证券", Type: "stock"}},
			OccurredAt:      "2026-07-20",
		},
		{Risks: []Signal{{Name: "券商板块", Type: "sector"}}, OccurredAt: "2026-07-19"},
	}

	// 2. 核对组名、逐条计数和完整成员列表。
	result := buildSignalStats(rows, groups)
	if len(result) != 1 {
		t.Fatalf("signals = %#v, want one grouped signal", result)
	}
	wantMembers := []string{"券商", "中信证券", "券商板块"}
	if result[0].Name != "证券行业" || result[0].RecommendationCount != 2 || result[0].RiskCount != 1 || result[0].Count != 3 {
		t.Fatalf("signal = %#v, want grouped counts 2-1-3", result[0])
	}
	if strings.Join(result[0].Members, ",") != strings.Join(wantMembers, ",") {
		t.Fatalf("members = %#v, want %#v", result[0].Members, wantMembers)
	}
}

// TestBuildSignalStatsReturnsEachMemberNetCount 验证概念组同时返回每个原始标的的净数。
// 输入：同组内分别出现纯推荐、推荐风险相抵和纯风险的三个标的。
// 输出：成员净数分别为正数、零和负数。
// 副作用：无。
func TestBuildSignalStatsReturnsEachMemberNetCount(t *testing.T) {
	// 1. 构造一个概念组及三种净数结果。
	groups := []SignalGroup{{
		ID: 1, Name: "证券行业", Type: "sector",
		Aliases: []string{"中信证券", "券商", "证券板块"},
	}}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "中信证券"}, {Name: "券商"}}},
		{Recommendations: []Signal{{Name: "券商"}}, Risks: []Signal{{Name: "券商"}, {Name: "证券板块"}}},
	}

	// 2. 核对每个标签使用自己的推荐减风险结果。
	result := buildSignalStats(rows, groups)
	if len(result) != 1 {
		t.Fatalf("signals = %#v, want one grouped signal", result)
	}
	got := result[0].MemberNetCounts
	if got["中信证券"] != 1 || got["券商"] != 1 || got["证券板块"] != -1 {
		t.Fatalf("member net counts = %#v, want 中信证券=1 券商=1 证券板块=-1", got)
	}
}

// TestBuildSignalStatsReturnsContinuousDailyNetHistory 验证概念组累计净数曲线按自然日延续。
// 输入：同一概念在首尾日期分别净推荐和净风险，中间日期只有其他概念。
// 输出：返回日期升序的 -1、-1、0 三个累计点，末点等于排行榜净数。
// 副作用：无。
func TestBuildSignalStatsReturnsContinuousDailyNetHistory(t *testing.T) {
	groups := []SignalGroup{
		{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商", "中信证券"}},
		{ID: 2, Name: "黄金", Type: "commodity", Aliases: []string{"黄金"}},
	}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "券商"}}, OccurredAt: "2026-07-20 09:00:00"},
		{Recommendations: []Signal{{Name: "黄金"}}, OccurredAt: "2026-07-19 09:00:00"},
		{Risks: []Signal{{Name: "中信证券"}}, OccurredAt: "2026-07-18 09:00:00"},
	}

	result := buildSignalStats(rows, groups)
	if len(result) != 2 {
		t.Fatalf("signals = %#v, want two groups", result)
	}
	var history []SignalNetPoint
	for _, item := range result {
		if item.Name == "证券行业" {
			history = item.NetHistory
		}
	}
	want := []SignalNetPoint{{Date: "2026-07-18", NetCount: -1}, {Date: "2026-07-19", NetCount: -1}, {Date: "2026-07-20", NetCount: 0}}
	if len(history) != len(want) {
		t.Fatalf("history = %#v, want %#v", history, want)
	}
	for index := range want {
		if history[index] != want[index] {
			t.Fatalf("history[%d] = %#v, want %#v", index, history[index], want[index])
		}
	}
	if history[len(history)-1].NetCount != result[0].RecommendationCount-result[0].RiskCount {
		t.Fatalf("last history net = %d, rank net = %d", history[len(history)-1].NetCount, result[0].RecommendationCount-result[0].RiskCount)
	}
}

// TestBuildSignalStatsUsesRequestedHistoryRange 验证报告日期窗口无信号日延续累计净数。
func TestBuildSignalStatsUsesRequestedHistoryRange(t *testing.T) {
	groups := []SignalGroup{{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}}}
	rows := []analysisRow{{Recommendations: []Signal{{Name: "券商"}}, OccurredAt: "2026-07-19 09:00:00"}}

	result := buildSignalStatsForDateRange(rows, groups, "2026-07-18", "2026-07-20")
	if len(result) != 1 {
		t.Fatalf("signals = %#v, want one group", result)
	}
	want := []SignalNetPoint{{Date: "2026-07-18", NetCount: 0}, {Date: "2026-07-19", NetCount: 1}, {Date: "2026-07-20", NetCount: 1}}
	for index := range want {
		if result[0].NetHistory[index] != want[index] {
			t.Fatalf("history[%d] = %#v, want %#v", index, result[0].NetHistory[index], want[index])
		}
	}
}

// TestBuildSignalStatsReconcilesUndatedSignals 验证无法落到具体日期的信号仍计入趋势基数。
func TestBuildSignalStatsReconcilesUndatedSignals(t *testing.T) {
	groups := []SignalGroup{{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}}}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "券商"}}, OccurredAt: "2026-07-19 09:00:00"},
		{Risks: []Signal{{Name: "券商"}}, OccurredAt: "invalid-date"},
	}

	result := buildSignalStatsForDateRange(rows, groups, "2026-07-18", "2026-07-20")
	want := []SignalNetPoint{{Date: "2026-07-18", NetCount: -1}, {Date: "2026-07-19", NetCount: 0}, {Date: "2026-07-20", NetCount: 0}}
	if len(result) != 1 || len(result[0].NetHistory) != len(want) {
		t.Fatalf("signals = %#v, want one group with %#v", result, want)
	}
	for index := range want {
		if result[0].NetHistory[index] != want[index] {
			t.Fatalf("history[%d] = %#v, want %#v", index, result[0].NetHistory[index], want[index])
		}
	}
	if result[0].NetHistory[len(want)-1].NetCount != result[0].RecommendationCount-result[0].RiskCount {
		t.Fatalf("last history net must equal rank net")
	}
}

// TestBuildSignalStatsPreservesRawAliasSpellings 验证概念成员保留实际出现的大小写写法。
// 输入：同一概念中规范化后相同的“AI硬件”和“ai硬件”。
// 输出：统计合并为一行，成员列表仍完整保留两种原文供页面筛选。
// 副作用：无。
func TestBuildSignalStatsPreservesRawAliasSpellings(t *testing.T) {
	// 1. 构造共享概念映射及两种实际文章写法。
	groups := []SignalGroup{{ID: 2, Name: "人工智能硬件", Type: "theme", Aliases: []string{"AI硬件"}}}
	rows := []analysisRow{{
		Recommendations: []Signal{{Name: "AI硬件", Type: "theme"}, {Name: "ai硬件", Type: "theme"}},
		OccurredAt:      "2026-07-20",
	}}

	// 2. 两次命中必须合并计数，但成员原文不能被大小写规范化去重。
	result := buildSignalStats(rows, groups)
	if len(result) != 1 || result[0].Count != 2 {
		t.Fatalf("signals = %#v, want one concept with two hits", result)
	}
	if strings.Join(result[0].Members, ",") != "AI硬件,ai硬件" {
		t.Fatalf("members = %#v, want both raw spellings", result[0].Members)
	}
}

// TestBuildSignalStatsCombinesUnmappedAliasesAtBottom 验证未归类标的不再各自占据信号榜行。
// 输入：一个已映射证券行业、一个已登记待归类别名和一个尚未映射名称。
// 输出：排行榜只包含证券行业与末尾的待归类行，并完整保留待归类成员。
// 副作用：无。
func TestBuildSignalStatsCombinesUnmappedAliasesAtBottom(t *testing.T) {
	// 1. 构造高频未知名称和低频已映射证券信号，确保排序规则会受到检验。
	groups := []SignalGroup{
		{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}},
		{ID: 2, Name: pendingSignalGroupName, Type: pendingSignalGroupType, Aliases: []string{"里子"}},
	}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "传统行业"}, {Name: "里子"}}, OccurredAt: "2026-07-20"},
		{Recommendations: []Signal{{Name: "传统行业"}, {Name: "里子"}}, OccurredAt: "2026-07-19"},
		{Risks: []Signal{{Name: "券商"}}, OccurredAt: "2026-07-18"},
	}

	// 2. 已登记和未登记的待归类名称合并成一行，固定排在已确认概念组之后。
	result := buildSignalStats(rows, groups)
	if len(result) != 2 || result[0].Name != "证券行业" || result[1].Name != pendingSignalGroupName {
		t.Fatalf("signals = %#v", result)
	}
	if result[1].Count != 4 || strings.Join(result[1].Members, ",") != "传统行业,里子" {
		t.Fatalf("pending = %#v", result[1])
	}
}

// TestAnalysisPromptTemplateKeepsFinalV9Rules 验证 Go 分析提示词同时完成文章判断和无模糊概念归类。
// 输入：页面展示使用的占位符提示词。
// 输出：结果导向示例、未来理由范围和推荐风险原因约束均完整存在。
// 副作用：无。
func TestAnalysisPromptTemplateKeepsFinalV9Rules(t *testing.T) {
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
		`"signal_classifications"`,
		"每个最终标的必须且只能有一条决策",
		"优先 reuse",
		`{"id":123,"name":"{现有概念组名称}","type":"sector"}`,
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
