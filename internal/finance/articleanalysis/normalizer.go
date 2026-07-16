package articleanalysis

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	allowedMoods = map[string]bool{
		"very_optimistic": true, "optimistic": true, "neutral": true,
		"pessimistic": true, "very_pessimistic": true, "unknown": true,
	}
	allowedPredictions = map[string]bool{"up": true, "down": true, "range": true, "unknown": true}
	resultOnlyPatterns = []string{"年初至今", "今年以来", "YTD", "收益率", "涨幅", "跌幅", "大涨", "暴涨", "领涨", "创新高"}
	forwardTerms       = []string{"预期", "预计", "未来", "后续", "有望", "可能", "风险", "压力", "催化", "估值", "基本面", "业绩", "供需", "政策", "配置", "受益", "承压", "修复", "回调", "下行", "上行"}
	recommendTerms     = []string{"推荐", "机会", "低估", "修复", "改善", "增长", "受益", "有望", "配置"}
	riskTerms          = []string{"风险", "不及预期", "承压", "下行", "恶化", "衰退", "下降", "亏损", "高估", "压力", "不确定"}
	percentPattern     = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?%`)
	bracketPattern     = regexp.MustCompile(`[（(].*?[）)]`)
	spacePattern       = regexp.MustCompile(`\s+`)
)

// NormalizeAnalysis 清洗模型结果并确保每个标的只有一个最终方向。
// 输入：value 是模型 JSON 解码后的结构。
// 输出：返回可直接写入 MySQL 的规范化结果。
// 副作用：无。
func NormalizeAnalysis(value AnalysisResult) AnalysisResult {
	// 1. 规范化摘要和短期市场枚举。
	value.Summary = truncateRunes(strings.TrimSpace(value.Summary), 1000)
	value.Market.Mood = normalizeMood(value.Market.Mood)
	value.Market.MoodReason = truncateRunes(strings.TrimSpace(value.Market.MoodReason), 240)
	value.Market.Prediction = normalizePrediction(value.Market.Prediction)
	value.Market.PredictionReason = truncateRunes(strings.TrimSpace(value.Market.PredictionReason), 240)

	// 2. 分别清洗、合并信号，再解决跨方向冲突。
	recommendations := mergeSignals(normalizeSignals(value.Recommendations))
	risks := mergeSignals(normalizeSignals(value.Risks))
	value.Recommendations, value.Risks = resolveSignalConflicts(recommendations, risks)
	return value
}

// normalizeMood 规范化市场氛围枚举并把谨慎归入中性。
// 输入：value 是模型枚举。
// 输出：返回允许值之一。
// 副作用：无。
func normalizeMood(value string) string {
	// 1. 按明确业务规则合并 cautious。
	value = strings.TrimSpace(value)
	if value == "cautious" {
		return "neutral"
	}
	if allowedMoods[value] {
		return value
	}
	return "unknown"
}

// normalizePrediction 规范化涨跌预测枚举。
// 输入：value 是模型枚举。
// 输出：返回 up、down、range 或 unknown。
// 副作用：无。
func normalizePrediction(value string) string {
	// 1. 无效值统一降级为 unknown。
	value = strings.TrimSpace(value)
	if allowedPredictions[value] {
		return value
	}
	return "unknown"
}

// normalizeSignals 清洗信号名称、类型和原因并过滤纯历史结果。
// 输入：items 是推荐或风险数组。
// 输出：最多返回 20 个有效信号。
// 副作用：无。
func normalizeSignals(items []Signal) []Signal {
	// 1. 逐项压缩名称和原因。
	if len(items) > 20 {
		items = items[:20]
	}
	results := make([]Signal, 0, len(items))
	for _, item := range items {
		item.Name = compactSignalName(item.Name)
		item.Type = truncateRunes(strings.TrimSpace(item.Type), 30)
		if item.Type == "" {
			item.Type = "other"
		}
		item.Reason = truncateRunes(strings.TrimSpace(item.Reason), 240)
		if item.Name == "" || isResultOnlySignal(item.Name, item.Reason) {
			continue
		}
		results = append(results, item)
	}
	return results
}

type mergedSignal struct {
	Signal
	mentions int
}

// mergeSignals 合并同一方向中的重复标的及原因。
// 输入：items 是已清洗信号。
// 输出：按第一次出现顺序返回合并结果。
// 副作用：无。
func mergeSignals(items []Signal) []mergedSignal {
	// 1. 使用名称索引合并次数和不重复原因。
	indexes := make(map[string]int)
	results := make([]mergedSignal, 0, len(items))
	for _, item := range items {
		index, exists := indexes[item.Name]
		if !exists {
			indexes[item.Name] = len(results)
			results = append(results, mergedSignal{Signal: item, mentions: 1})
			continue
		}
		results[index].mentions++
		if results[index].Type == "other" && item.Type != "other" {
			results[index].Type = item.Type
		}
		results[index].Reason = joinReasons(results[index].Reason, item.Reason)
	}
	return results
}

// resolveSignalConflicts 确保同一标的不同时出现在推荐和风险两侧。
// 输入：recommendations 和 risks 是各自已合并信号。
// 输出：返回最终推荐和风险数组。
// 副作用：无。
func resolveSignalConflicts(recommendations, risks []mergedSignal) ([]Signal, []Signal) {
	// 1. 建立风险名称索引并逐个裁决推荐冲突。
	riskByName := make(map[string]mergedSignal)
	for _, item := range risks {
		riskByName[item.Name] = item
	}
	handled := make(map[string]bool)
	resolvedRecommendations := make([]Signal, 0)
	resolvedRisks := make([]Signal, 0)
	for _, recommendation := range recommendations {
		risk, exists := riskByName[recommendation.Name]
		if !exists {
			resolvedRecommendations = append(resolvedRecommendations, recommendation.Signal)
			continue
		}
		handled[risk.Name] = true
		if signalScore(risk, riskTerms) > signalScore(recommendation, recommendTerms) {
			risk.Reason = joinReasons(risk.Reason, recommendation.Reason)
			resolvedRisks = append(resolvedRisks, risk.Signal)
		} else {
			recommendation.Reason = joinReasons(recommendation.Reason, risk.Reason)
			resolvedRecommendations = append(resolvedRecommendations, recommendation.Signal)
		}
	}

	// 2. 补充没有发生冲突的风险信号。
	for _, risk := range risks {
		if !handled[risk.Name] {
			resolvedRisks = append(resolvedRisks, risk.Signal)
		}
	}
	return resolvedRecommendations, resolvedRisks
}

// signalScore 根据提及次数和方向词计算冲突裁决分数。
// 输入：signal 是合并信号，terms 是对应方向词。
// 输出：返回整数分数。
// 副作用：无。
func signalScore(signal mergedSignal, terms []string) int {
	// 1. 每次提及计两分，每个方向词计一分。
	score := signal.mentions * 2
	for _, term := range terms {
		if strings.Contains(signal.Reason, term) {
			score++
		}
	}
	return score
}

// compactSignalName 把模型长名称压缩为适合表格展示的核心标的。
// 输入：value 是模型名称。
// 输出：返回最多 12 个字符的名称。
// 副作用：无。
func compactSignalName(value string) string {
	// 1. 去括号、符号、空白和常见分隔后的附加标的。
	name := bracketPattern.ReplaceAllString(strings.TrimSpace(value), "")
	name = strings.NewReplacer("【", "", "】", "", "[", "", "]", "", "《", "", "》", "", "\"", "", "'", "").Replace(name)
	name = spacePattern.ReplaceAllString(name, "")
	name = strings.Trim(name, "，,。.!！?？、;；:：|/\\_-—")
	for _, separator := range []string{"、", ",", "，", ";", "；", "|", "以及", "和", "与", "及"} {
		if index := strings.Index(name, separator); index > 0 {
			name = name[:index]
		}
	}
	return truncateRunes(name, 12)
}

// isResultOnlySignal 判断信号是否只描述已经发生的涨跌结果。
// 输入：name 和 reason 是信号文本。
// 输出：缺少未来判断且命中历史结果模式时返回 true。
// 副作用：无。
func isResultOnlySignal(name, reason string) bool {
	// 1. 没有结果模式时直接保留。
	text := name + reason
	hasResult := percentPattern.MatchString(text)
	for _, pattern := range resultOnlyPatterns {
		hasResult = hasResult || strings.Contains(text, pattern)
	}
	if !hasResult {
		return false
	}

	// 2. 存在明确未来判断词时仍保留。
	for _, term := range forwardTerms {
		if strings.Contains(text, term) {
			return false
		}
	}
	return true
}

// joinReasons 拼接两个不重复原因并限制长度。
// 输入：left 和 right 是短原因。
// 输出：返回分号连接的最多 240 字原因。
// 副作用：无。
func joinReasons(left, right string) string {
	// 1. 去空白并避免重复文本。
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" {
		return truncateRunes(right, 240)
	}
	if right == "" || right == left {
		return truncateRunes(left, 240)
	}
	return truncateRunes(left+"；"+right, 240)
}

// truncateRunes 按 Unicode 字符而非字节安全截断文本。
// 输入：value 是任意 UTF-8 文本，limit 是最大字符数。
// 输出：返回不超过限制的合法 UTF-8 文本。
// 副作用：无。
func truncateRunes(value string, limit int) string {
	// 1. 短文本直接返回，长文本按 rune 截断。
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
