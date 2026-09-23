package articleanalysis

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AnalysisSummary 构建投资文章分析页面摘要。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回文章和已分析计数；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) AnalysisSummary(ctx context.Context) (PageSummary, error) {
	// 1. 读取统一计数口径。
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return PageSummary{}, fmt.Errorf("读取文章分析摘要: %w", err)
	}

	// 2. 组装前端状态卡片。
	return PageSummary{
		Title:       "投资文章分析",
		Description: "统计最近资讯里的标的信号、市场氛围和涨跌预测。",
		Metrics: []PageMetric{
			{Label: "文章", Value: strconv.Itoa(counts.ArticleCount), Detail: "已入库文章", Status: "normal"},
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "结构化模型结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// FetchSummary 构建投资文章抓取页面摘要。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回来源、文章、待分析和已分析计数；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) FetchSummary(ctx context.Context) (PageSummary, error) {
	// 1. 读取统一计数口径。
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return PageSummary{}, fmt.Errorf("读取文章抓取摘要: %w", err)
	}

	// 2. 组装抓取页状态卡片。
	return PageSummary{
		Title:       "投资文章抓取",
		Description: "管理微信读书公众号，读取新文章并触发结构化模型分析。",
		Metrics: []PageMetric{
			{Label: "来源", Value: strconv.Itoa(counts.SourceCount), Detail: "启用的信息源", Status: "normal"},
			{Label: "文章", Value: strconv.Itoa(counts.ArticleCount), Detail: "已入库文章", Status: "normal"},
			{Label: "待分析", Value: strconv.Itoa(counts.PendingCount), Detail: "抓取后等待模型处理", Status: "normal"},
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "结构化模型结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// Sources 返回页面展示的信息源列表。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回全部来源；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Sources(ctx context.Context) ([]Source, error) {
	// 1. 页面需要同时看到未配置来源状态。
	return s.repository.Sources(ctx, false)
}

// Articles 返回指定天数内已分析文章。
// 输入：ctx 控制查询，days 和 limit 限制范围。
// 输出：返回文章列表；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Articles(ctx context.Context, days, limit int) ([]ArticleItem, error) {
	// 1. 复用仓储层受限查询。
	return s.repository.Articles(ctx, days, limit)
}

// Detail 返回单篇文章详情。
// 输入：ctx 控制查询，articleID 是文章主键。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Detail(ctx context.Context, articleID int64) (*ArticleDetail, error) {
	// 1. 复用仓储层详情映射。
	return s.repository.Detail(ctx, articleID)
}

// UpdatePromptFeedback 保存管理员修正意见并返回最新详情。
// 输入：ctx 控制写入，articleID 是文章主键，feedback 是修正意见。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：写入 PostgreSQL。
func (s *Service) UpdatePromptFeedback(ctx context.Context, articleID int64, feedback string) (*ArticleDetail, error) {
	// 1. 由仓储层统一截断并更新反馈。
	return s.repository.UpdatePromptFeedback(ctx, articleID, feedback)
}

// Report 构建信号榜和短期市场分布。
// 输入：ctx 控制查询，targetDays 默认 90，marketDays 默认 3。
// 输出：返回完整分析报告；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Report(ctx context.Context, targetDays, marketDays int) (Report, error) {
	// 1. 分别读取信号榜和市场判断的独立日期范围。
	if targetDays < 1 {
		targetDays = DefaultTargetDays
	}
	if marketDays < 1 {
		marketDays = DefaultMarketDays
	}
	targetRows, err := s.repository.analysisRows(ctx, targetDays)
	if err != nil {
		return Report{}, fmt.Errorf("读取信号榜统计: %w", err)
	}
	marketRows, err := s.repository.analysisRows(ctx, marketDays)
	if err != nil {
		return Report{}, fmt.Errorf("读取短期市场统计: %w", err)
	}
	groups, err := s.repository.SignalGroups(ctx)
	if err != nil {
		return Report{}, err
	}

	// 2. 聚合推荐、风险及市场枚举并附带当前模型说明。
	model, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return Report{}, err
	}
	historyEnd := dateOnly(shanghaiNowText())
	historyEndTime, _ := time.Parse("2006-01-02", historyEnd)
	historyStart := historyEndTime.AddDate(0, 0, -(targetDays - 1)).Format("2006-01-02")
	return Report{
		AnalysisModel:          model.Model,
		AnalysisPrompt:         AnalysisPromptTemplate(),
		PromptVersion:          PromptVersion,
		Signals:                buildSignalStatsForDateRange(targetRows, groups, historyStart, historyEnd),
		MoodDistribution:       buildDistribution(marketRows, true),
		PredictionDistribution: buildDistribution(marketRows, false),
	}, nil
}

type signalAccumulator struct {
	SignalStat
	LatestAt   string
	memberKeys map[string]struct{}
}

type aggregatedSignal struct {
	Name     string
	Type     string
	Count    int
	LatestAt string
}

// buildSignalStats 按概念组把推荐和风险合并为每个标的一行。
// 输入：rows 是目标天数内分析行，groups 是已持久化的概念组和别名。
// 输出：按总次数和最近出现时间倒序返回信号榜。
// 副作用：无。
func buildSignalStats(rows []analysisRow, groups []SignalGroup) []SignalStat {
	return buildSignalStatsForDateRange(rows, groups, "", "")
}

// buildSignalStatsForDateRange 按概念组聚合信号，并可固定每日趋势的起止日期。
// 输入：rows 和 groups 构成排行榜，historyStart 与 historyEnd 为空时使用数据自身边界。
// 输出：返回带连续每日净数曲线的信号榜。
// 副作用：无。
func buildSignalStatsForDateRange(rows []analysisRow, groups []SignalGroup, historyStart, historyEnd string) []SignalStat {
	// 1. 建立别名索引，并沿用旧服务顺序分别聚合推荐、风险。
	groupIndex := buildSignalGroupIndex(groups)
	recommendations := aggregateSignals(rows, true)
	risks := aggregateSignals(rows, false)

	// 2. 推荐聚合项先进入概念榜，风险项补充计数或追加纯风险概念。
	grouped := make(map[string]*signalAccumulator)
	ordered := make([]*signalAccumulator, 0, len(recommendations)+len(risks))
	merge := func(signals []aggregatedSignal, recommendation bool) {
		for _, signal := range signals {
			groupKey, groupName, groupType := signalGroupIdentity(signal.Name, groupIndex)
			item, exists := grouped[groupKey]
			if !exists {
				item = &signalAccumulator{
					SignalStat: SignalStat{
						Name:            groupName,
						Type:            groupType,
						Members:         []string{},
						MemberNetCounts: map[string]int{},
					},
					LatestAt: signal.LatestAt, memberKeys: make(map[string]struct{}),
				}
				grouped[groupKey] = item
				ordered = append(ordered, item)
			}
			memberKey := strings.TrimSpace(signal.Name)
			if _, exists := item.memberKeys[memberKey]; !exists {
				item.memberKeys[memberKey] = struct{}{}
				item.Members = append(item.Members, strings.TrimSpace(signal.Name))
			}
			if item.Type == "other" && signal.Type != "" && signal.Type != "other" {
				item.Type = signal.Type
			}
			if recommendation {
				item.RecommendationCount += signal.Count
				item.MemberNetCounts[memberKey] += signal.Count
			} else {
				item.RiskCount += signal.Count
				item.MemberNetCounts[memberKey] -= signal.Count
			}
			item.Count += signal.Count
			if signal.LatestAt > item.LatestAt {
				item.LatestAt = signal.LatestAt
			}
		}
	}
	merge(recommendations, true)
	merge(risks, false)
	applySignalNetHistory(rows, groupIndex, grouped, historyStart, historyEnd)

	// 3. 稳定排序保证完全相同的总数和日期保留聚合插入顺序。
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Name == pendingSignalGroupName || ordered[right].Name == pendingSignalGroupName {
			return ordered[right].Name == pendingSignalGroupName && ordered[left].Name != pendingSignalGroupName
		}
		if ordered[left].Count == ordered[right].Count {
			return ordered[left].LatestAt > ordered[right].LatestAt
		}
		return ordered[left].Count > ordered[right].Count
	})
	results := make([]SignalStat, 0, len(ordered))
	for _, item := range ordered {
		results = append(results, item.SignalStat)
	}
	return results
}

// signalGroupIdentity 返回原始标的所属的稳定概念组键和展示信息。
// 输入：name 是原始标的名，groupIndex 是别名映射。
// 输出：返回内部聚合键、概念组名和类型；未映射名称返回统一兜底组。
// 副作用：无。
func signalGroupIdentity(name string, groupIndex map[string]SignalGroup) (string, string, string) {
	// 1. 未映射名称和旧待归类别名沿用同一兜底身份。
	groupKey := "pending"
	groupName, groupType := pendingSignalGroupName, pendingSignalGroupType
	group, exists := groupIndex[normalizeSignalAlias(name)]
	if !exists || group.Name == pendingSignalGroupName || group.Type == pendingSignalGroupType {
		return groupKey, groupName, groupType
	}

	// 2. 正式概念组使用数据库 ID 和规范名称组成稳定键。
	groupKey = fmt.Sprintf("group:%d:%s", group.ID, normalizeSignalAlias(group.Name))
	groupName = group.Name
	if strings.TrimSpace(group.Type) != "" {
		groupType = group.Type
	}
	return groupKey, groupName, groupType
}

// applySignalNetHistory 为每个概念组生成按自然日连续的累计推荐减风险净数。
// 输入：rows 是文章信号行，groupIndex 和 grouped 使用排行榜映射，日期参数可固定展示窗口。
// 输出：无返回值，直接补充每个排行榜项的 NetHistory。
// 副作用：修改 grouped 中的内存统计，不读写数据库。
func applySignalNetHistory(
	rows []analysisRow,
	groupIndex map[string]SignalGroup,
	grouped map[string]*signalAccumulator,
	historyStart string,
	historyEnd string,
) {
	// 1. 逐篇按发生日期累计概念组推荐和风险净数，并记录有效日期边界。
	daily := make(map[string]map[string]int)
	minDate, maxDate := "", ""
	addSignals := func(date string, signals []Signal, delta int) {
		for _, signal := range signals {
			if strings.TrimSpace(signal.Name) == "" {
				continue
			}
			groupKey, _, _ := signalGroupIdentity(signal.Name, groupIndex)
			if _, exists := grouped[groupKey]; !exists {
				continue
			}
			if daily[groupKey] == nil {
				daily[groupKey] = make(map[string]int)
			}
			daily[groupKey][date] += delta
		}
	}
	for _, row := range rows {
		date := dateOnly(row.OccurredAt)
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		if minDate == "" || date < minDate {
			minDate = date
		}
		if maxDate == "" || date > maxDate {
			maxDate = date
		}
		addSignals(date, row.Recommendations, 1)
		addSignals(date, row.Risks, -1)
	}
	if _, err := time.Parse("2006-01-02", historyStart); err == nil {
		minDate = historyStart
	}
	if _, err := time.Parse("2006-01-02", historyEnd); err == nil {
		maxDate = historyEnd
	}
	if minDate == "" || maxDate == "" || minDate > maxDate {
		return
	}

	// 2. 逐日累加净变化；无信号日期延续前一天净数，末点与排行榜当前净数一致。
	start, _ := time.Parse("2006-01-02", minDate)
	end, _ := time.Parse("2006-01-02", maxDate)
	for groupKey, item := range grouped {
		item.NetHistory = make([]SignalNetPoint, 0, int(end.Sub(start).Hours()/24)+1)
		visibleNetChange := 0
		for date, netChange := range daily[groupKey] {
			if date >= minDate && date <= maxDate {
				visibleNetChange += netChange
			}
		}
		runningNetCount := item.RecommendationCount - item.RiskCount - visibleNetChange
		for current := start; !current.After(end); current = current.AddDate(0, 0, 1) {
			date := current.Format("2006-01-02")
			runningNetCount += daily[groupKey][date]
			item.NetHistory = append(item.NetHistory, SignalNetPoint{Date: date, NetCount: runningNetCount})
		}
	}
}

// buildSignalGroupIndex 把概念组别名转换为规范化名称索引。
// 输入：groups 是数据库读取的概念组和全部别名。
// 输出：返回原始名称到概念组的只读映射。
// 副作用：无。
func buildSignalGroupIndex(groups []SignalGroup) map[string]SignalGroup {
	// 1. 为每个非空别名登记所属概念组，重复别名保留首个有效配置。
	index := make(map[string]SignalGroup)
	for _, group := range groups {
		for _, alias := range group.Aliases {
			key := normalizeSignalAlias(alias)
			if key == "" {
				continue
			}
			if _, exists := index[key]; !exists {
				index[key] = group
			}
		}
	}
	return index
}

// normalizeSignalAlias 规范化别名查找键，不改变页面展示原文。
// 输入：name 是模型返回或数据库保存的原始标的名称。
// 输出：返回去除首尾空白并转为小写的稳定键。
// 副作用：无。
func normalizeSignalAlias(name string) string {
	// 1. 只处理无语义差异的大小写和首尾空白，近义词交给概念映射。
	return strings.ToLower(strings.TrimSpace(name))
}

// aggregateSignals 按名称和类型聚合推荐或风险信号。
// 输入：rows 是时间倒序分析行，recommendation 选择推荐或风险数组。
// 输出：按次数、最近日期倒序返回聚合项，完全同分时保留首次出现顺序。
// 副作用：无。
func aggregateSignals(rows []analysisRow, recommendation bool) []aggregatedSignal {
	// 1. 使用结构化键累计次数，并用切片保留仓储结果中的首次出现顺序。
	type signalKey struct {
		name string
		kind string
	}
	grouped := make(map[signalKey]*aggregatedSignal)
	ordered := make([]*aggregatedSignal, 0)
	for _, row := range rows {
		signals := row.Risks
		if recommendation {
			signals = row.Recommendations
		}
		for _, signal := range signals {
			name := strings.TrimSpace(signal.Name)
			if name == "" {
				continue
			}
			kind := strings.TrimSpace(signal.Type)
			if kind == "" {
				kind = "other"
			}
			key := signalKey{name: name, kind: kind}
			item, exists := grouped[key]
			if !exists {
				item = &aggregatedSignal{Name: name, Type: kind, LatestAt: row.OccurredAt}
				grouped[key] = item
				ordered = append(ordered, item)
			}
			item.Count++
			if row.OccurredAt > item.LatestAt {
				item.LatestAt = row.OccurredAt
			}
		}
	}

	// 2. 稳定排序复刻旧服务按次数和最近日期倒序的聚合列表。
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Count == ordered[right].Count {
			return ordered[left].LatestAt > ordered[right].LatestAt
		}
		return ordered[left].Count > ordered[right].Count
	})
	results := make([]aggregatedSignal, 0, len(ordered))
	for _, item := range ordered {
		results = append(results, *item)
	}
	return results
}

var moodDistributionNames = []string{"very_optimistic", "optimistic", "neutral", "pessimistic", "very_pessimistic", "unknown"}
var predictionDistributionNames = []string{"up", "range", "down", "unknown"}

// buildDistribution 统计市场氛围或涨跌预测分布，并保留计数为零的固定类别。
// 输入：rows 是市场天数内分析行，mood 控制统计字段。
// 输出：按固定业务顺序返回完整分布。
// 副作用：无。
func buildDistribution(rows []analysisRow, mood bool) []DistributionItem {
	// 1. 规范化枚举并累计次数。
	counts := make(map[string]int)
	for _, row := range rows {
		key := normalizePrediction(row.Prediction)
		if mood {
			key = normalizeMood(row.MarketMood)
		}
		counts[key]++
	}
	names := predictionDistributionNames
	if mood {
		names = moodDistributionNames
	}

	// 2. 固定类别即使计数为零也返回，避免页面布局随文章内容变化。
	results := make([]DistributionItem, 0, len(names))
	for _, name := range names {
		results = append(results, DistributionItem{Name: name, Count: counts[name]})
	}
	return results
}
