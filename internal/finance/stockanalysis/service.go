package stockanalysis

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/howiedata/aowugong-go/internal/money"
)

// Service 负责把仓位快照聚合为前端可直接消费的分析报告。
type Service struct {
	repository *Repository
}

// NewService 创建股票仓位分析服务。
// 输入：repository 提供受限 MySQL 查询。
// 输出：返回分析服务。
// 副作用：无。
func NewService(repository *Repository) *Service {
	// 1. 保存显式仓储依赖。
	return &Service{repository: repository}
}

// Summary 生成股票仓位分析页顶部摘要。
// 输入：ctx 控制 MySQL 查询。
// 输出：返回四项指标；失败时返回带业务上下文的错误。
// 副作用：只读 MySQL。
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	// 1. 复用完整报告，保证摘要和图表口径一致。
	report, err := s.Report(ctx, 500)
	if err != nil {
		return Summary{}, fmt.Errorf("生成股票仓位摘要: %w", err)
	}

	// 2. 无数据时返回稳定占位指标。
	latestDate := "未知"
	values := []string{"-", "-", "-", "0"}
	if report.Latest != nil {
		latestDate = report.Latest.SnapshotDate
		values = []string{
			formatMoneyText(report.Latest.TotalAsset),
			formatSignedMoneyText(report.Changes.TotalAssetChange),
			report.Latest.PositionPercent + "%",
			strconv.Itoa(report.Latest.AccountCount),
		}
	}
	return Summary{
		Title:       "股票仓位分析",
		Description: "基于每次导入的仓位截图，观察资产、市值、现金、总仓位和持仓分布。",
		Metrics: []Metric{
			{Label: "最新总资产", Value: values[0], Detail: "记录日期 " + latestDate, Status: "normal"},
			{Label: "资产变化", Value: values[1], Detail: "相对首个记录日，未扣除出入金", Status: "normal"},
			{Label: "总仓位", Value: values[2], Detail: "综合所有账户，总市值 / 总资产", Status: "normal"},
			{Label: "账户数", Value: values[3], Detail: fmt.Sprintf("共 %d 条快照", report.SnapshotCount), Status: "normal"},
		},
	}, nil
}

// Report 生成股票仓位完整分析报告。
// 输入：ctx 控制 MySQL 查询，limit 限制最近快照数量。
// 输出：返回趋势、账户、持仓、洞察和变化；失败时返回错误。
// 副作用：只读 MySQL。
func (s *Service) Report(ctx context.Context, limit int) (Report, error) {
	// 1. 读取受限快照并聚合时间线和账户。
	rows, err := s.repository.snapshots(ctx, limit)
	if err != nil {
		return Report{}, fmt.Errorf("读取股票仓位快照: %w", err)
	}
	timeline := buildTimeline(rows)
	accounts := buildAccounts(rows)
	report := Report{
		Timeline:      timeline,
		Accounts:      accounts,
		Insights:      buildInsights(timeline),
		Ideas:         analysisIdeas(),
		SnapshotCount: len(rows),
		DateCount:     len(timeline),
		Changes:       Changes{TotalAssetChange: "0.00", MarketValueChange: "0.00", AvailableCashChange: "0.00", DailyTotalAssetChange: "0.00"},
	}

	// 2. 有时间线时补充首尾指针和金额变化。
	if len(timeline) > 0 {
		report.First = &report.Timeline[0]
		report.Latest = &report.Timeline[len(report.Timeline)-1]
		if len(report.Timeline) > 1 {
			report.Previous = &report.Timeline[len(report.Timeline)-2]
		}
		report.Changes = calculateChanges(report.First, report.Previous, report.Latest)

		// 3. 只按最新日期读取持仓，避免无条件扫描明细大表。
		holdings, err := s.repository.holdings(ctx, report.Latest.SnapshotDate)
		if err != nil {
			return Report{}, fmt.Errorf("读取股票持仓分布: %w", err)
		}
		report.Holdings = buildHoldingDistribution(holdings, report.Latest)
	}
	if report.Holdings == nil {
		report.Holdings = []HoldingDistribution{}
	}
	return report, nil
}

type aggregate struct {
	TotalAsset    int64
	MarketValue   int64
	AvailableCash int64
	OtherAmount   int64
	AccountCount  int
}

// buildTimeline 按日期聚合账户快照并计算首日和单日变化。
// 输入：rows 是按日期正序排列的账户快照。
// 输出：返回前端趋势图时间线。
// 副作用：无。
func buildTimeline(rows []snapshotRow) []TimelinePoint {
	// 1. 按日期合计资产，并把标准券从市值移入现金。
	byDate := make(map[string]*aggregate)
	dates := make([]string, 0)
	for _, row := range rows {
		item, exists := byDate[row.SnapshotDate]
		if !exists {
			item = &aggregate{}
			byDate[row.SnapshotDate] = item
			dates = append(dates, row.SnapshotDate)
		}
		item.TotalAsset += row.TotalAssetCents
		item.MarketValue += row.MarketValueCents - row.CashEquivalentCents
		item.AvailableCash += row.AvailableCashCents + row.CashEquivalentCents
		item.OtherAmount += row.OtherAmountCents
		item.AccountCount++
	}
	sort.Strings(dates)

	// 2. 生成变化字段和格式化金额。
	result := make([]TimelinePoint, 0, len(dates))
	var first, previous int64
	for index, date := range dates {
		item := byDate[date]
		if index == 0 {
			first = item.TotalAsset
			previous = item.TotalAsset
		}
		daily := item.TotalAsset - previous
		result = append(result, TimelinePoint{
			SnapshotDate: date, TotalAsset: money.FormatCents(item.TotalAsset, false),
			MarketValue: money.FormatCents(item.MarketValue, false), AvailableCash: money.FormatCents(item.AvailableCash, false),
			OtherAmount: money.FormatCents(item.OtherAmount, false), PositionPercent: formatPercent(item.MarketValue, item.TotalAsset),
			DailyChange: money.FormatCents(daily, false), CumulativeChange: money.FormatCents(item.TotalAsset-first, false),
			AccountCount: item.AccountCount,
		})
		previous = item.TotalAsset
	}
	return result
}

// buildAccounts 按账户聚合首个、上一个和最新快照。
// 输入：rows 是按日期正序排列的账户快照。
// 输出：按最新总资产倒序返回账户摘要。
// 副作用：无。
func buildAccounts(rows []snapshotRow) []AccountSummary {
	// 1. 按账户后四位归组。
	groups := make(map[string][]snapshotRow)
	for _, row := range rows {
		groups[row.AccountSuffix] = append(groups[row.AccountSuffix], row)
	}

	// 2. 计算每个账户的最新金额和变化。
	result := make([]AccountSummary, 0, len(groups))
	for suffix, group := range groups {
		first := group[0]
		latest := group[len(group)-1]
		previous := latest
		if len(group) > 1 {
			previous = group[len(group)-2]
		}
		market := latest.MarketValueCents - latest.CashEquivalentCents
		cash := latest.AvailableCashCents + latest.CashEquivalentCents
		alias := latest.AccountAlias
		if alias == "" {
			alias = "**" + suffix
		}
		result = append(result, AccountSummary{
			AccountSuffix: suffix, AccountAlias: alias, BrokerName: latest.BrokerName, SnapshotDate: latest.SnapshotDate,
			TotalAsset: money.FormatCents(latest.TotalAssetCents, false), MarketValue: money.FormatCents(market, false),
			AvailableCash: money.FormatCents(cash, false), OtherAmount: money.FormatCents(latest.OtherAmountCents, false),
			PositionPercent:  formatPercent(market, latest.TotalAssetCents),
			DailyChange:      money.FormatCents(latest.TotalAssetCents-previous.TotalAssetCents, false),
			CumulativeChange: money.FormatCents(latest.TotalAssetCents-first.TotalAssetCents, false),
		})
	}

	// 3. 保持资产最多的账户在前。
	sort.Slice(result, func(left, right int) bool {
		leftCents, _ := money.ParseCents(result[left].TotalAsset)
		rightCents, _ := money.ParseCents(result[right].TotalAsset)
		return leftCents > rightCents
	})
	return result
}

// buildHoldingDistribution 合并证券持仓与调整后的现金分布。
// 输入：rows 是最新日证券聚合，latest 是组合资产点。
// 输出：按市值倒序返回分布，标准券不作为证券展示。
// 副作用：无。
func buildHoldingDistribution(rows []holdingRow, latest *TimelinePoint) []HoldingDistribution {
	// 1. 把非现金等价证券转换为前端模型。
	totalCents, _ := money.ParseCents(latest.TotalAsset)
	result := make([]HoldingDistribution, 0, len(rows)+1)
	for _, row := range rows {
		if row.SecurityName == "标准券" {
			continue
		}
		result = append(result, HoldingDistribution{
			SecurityName: row.SecurityName, MarketValue: money.FormatCents(row.MarketCents, false), Quantity: row.Quantity,
			WeightPercent: formatPercent(row.MarketCents, totalCents), AccountCount: row.AccountCount, Accounts: row.Accounts,
		})
	}

	// 2. 把已经包含标准券的可用资金作为现金维度。
	cashCents, _ := money.ParseCents(latest.AvailableCash)
	if cashCents > 0 {
		result = append(result, HoldingDistribution{
			SecurityName: "现金", MarketValue: money.FormatCents(cashCents, false),
			WeightPercent: formatPercent(cashCents, totalCents), AccountCount: latest.AccountCount, Accounts: "可用资金",
		})
	}
	sort.Slice(result, func(left, right int) bool {
		leftCents, _ := money.ParseCents(result[left].MarketValue)
		rightCents, _ := money.ParseCents(result[right].MarketValue)
		return leftCents > rightCents
	})
	return result
}

// calculateChanges 计算首尾和最近两个时间点之间的金额变化。
// 输入：first、previous 和 latest 是时间线关键点。
// 输出：返回四项变化金额。
// 副作用：无。
func calculateChanges(first, previous, latest *TimelinePoint) Changes {
	// 1. 将格式化金额恢复为整数分后计算。
	firstTotal, _ := money.ParseCents(first.TotalAsset)
	latestTotal, _ := money.ParseCents(latest.TotalAsset)
	firstMarket, _ := money.ParseCents(first.MarketValue)
	latestMarket, _ := money.ParseCents(latest.MarketValue)
	firstCash, _ := money.ParseCents(first.AvailableCash)
	latestCash, _ := money.ParseCents(latest.AvailableCash)
	previousTotal := latestTotal
	if previous != nil {
		previousTotal, _ = money.ParseCents(previous.TotalAsset)
	}
	return Changes{
		TotalAssetChange:      money.FormatCents(latestTotal-firstTotal, false),
		MarketValueChange:     money.FormatCents(latestMarket-firstMarket, false),
		AvailableCashChange:   money.FormatCents(latestCash-firstCash, false),
		DailyTotalAssetChange: money.FormatCents(latestTotal-previousTotal, false),
	}
}

// buildInsights 根据组合时间线生成四个解释性指标。
// 输入：timeline 是按日期正序的组合数据。
// 输出：无数据返回空数组，否则返回高点、仓位、现金和节奏提示。
// 副作用：无。
func buildInsights(timeline []TimelinePoint) []Insight {
	// 1. 空数据不生成推断。
	if len(timeline) == 0 {
		return []Insight{}
	}

	// 2. 扫描资产高点和仓位区间。
	highest := timeline[0]
	lowestPosition, highestPosition := timeline[0], timeline[0]
	for _, point := range timeline[1:] {
		if compareMoney(point.TotalAsset, highest.TotalAsset) > 0 {
			highest = point
		}
		if compareDecimal(point.PositionPercent, lowestPosition.PositionPercent) < 0 {
			lowestPosition = point
		}
		if compareDecimal(point.PositionPercent, highestPosition.PositionPercent) > 0 {
			highestPosition = point
		}
	}
	latest := timeline[len(timeline)-1]
	return []Insight{
		{Title: "历史最高资产", Value: formatMoneyText(highest.TotalAsset), Detail: highest.SnapshotDate},
		{Title: "总仓位区间", Value: lowestPosition.PositionPercent + "% - " + highestPosition.PositionPercent + "%", Detail: "按每次上传后的总市值 / 总资产计算"},
		{Title: "现金缓冲", Value: formatPercentText(latest.AvailableCash, latest.TotalAsset), Detail: "最新可用资金 " + formatMoneyText(latest.AvailableCash)},
		{Title: "记录节奏", Value: cadenceValue(timeline), Detail: cadenceDetail(timeline)},
	}
}

// analysisIdeas 返回页面沿用的分析扩展方向。
// 输入：无。
// 输出：返回五项固定说明。
// 副作用：无。
func analysisIdeas() []AnalysisIdea {
	// 1. 返回现有页面展示内容。
	return []AnalysisIdea{
		{Title: "真实收益曲线", Description: "接入银证转账或手工录入出入金后，用净值法剔除转入转出影响。"},
		{Title: "账户贡献拆分", Description: "按两个账户分别看资产变化，判断哪个账户贡献主要波动。"},
		{Title: "仓位预警", Description: "设置仓位上限、现金下限，超过阈值后在页面和通知里提示。"},
		{Title: "指数对比", Description: "把组合资产变化和主要指数走势放在同一张图里。"},
		{Title: "回撤统计", Description: "记录距离历史高点的跌幅，区分正常波动和需要复盘的回撤。"},
	}
}

// formatPercent 按金额分计算并格式化百分比。
// 输入：part 和 total 是整数分。
// 输出：返回两位小数百分比，不包含百分号。
// 副作用：无。
func formatPercent(part, total int64) string {
	// 1. 分母为零时返回稳定零值。
	if total == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", float64(part)*100/float64(total))
}

// formatMoneyText 把普通金额文本格式化为带千分位的两位小数。
// 输入：value 是十进制金额文本。
// 输出：解析成功时返回千分位金额，失败时返回原值。
// 副作用：无。
func formatMoneyText(value string) string {
	// 1. 复用项目唯一金额转换入口。
	cents, err := money.ParseCents(value)
	if err != nil {
		return value
	}
	return money.FormatCents(cents, true)
}

// formatSignedMoneyText 把金额文本格式化为显式正负金额。
// 输入：value 是十进制金额文本。
// 输出：正数带加号，其余保留金额符号。
// 副作用：无。
func formatSignedMoneyText(value string) string {
	// 1. 解析金额并添加正数前缀。
	cents, err := money.ParseCents(value)
	if err != nil {
		return value
	}
	prefix := ""
	if cents > 0 {
		prefix = "+"
	}
	return prefix + money.FormatCents(cents, true)
}

// formatPercentText 根据两个金额文本计算百分比。
// 输入：part 和 total 是十进制金额文本。
// 输出：返回带百分号的两位小数文本。
// 副作用：无。
func formatPercentText(part, total string) string {
	// 1. 转换金额并复用统一百分比格式。
	partCents, _ := money.ParseCents(part)
	totalCents, _ := money.ParseCents(total)
	return formatPercent(partCents, totalCents) + "%"
}

// cadenceValue 计算最近两个记录点的日期间隔展示值。
// 输入：timeline 是按日期正序的时间线。
// 输出：返回节奏标题。
// 副作用：无。
func cadenceValue(timeline []TimelinePoint) string {
	// 1. 单点数据使用原页面引导语。
	if len(timeline) < 2 {
		return "每周上传也可以"
	}
	latest, _ := time.Parse(time.DateOnly, timeline[len(timeline)-1].SnapshotDate)
	previous, _ := time.Parse(time.DateOnly, timeline[len(timeline)-2].SnapshotDate)
	days := int(latest.Sub(previous).Hours() / 24)
	if days <= 0 {
		return "同日更新"
	}
	return fmt.Sprintf("%d 天", days)
}

// cadenceDetail 返回与节奏标题配套的解释。
// 输入：timeline 是按日期正序的时间线。
// 输出：返回节奏说明。
// 副作用：无。
func cadenceDetail(timeline []TimelinePoint) string {
	// 1. 区分单点引导和已有间隔。
	if len(timeline) < 2 {
		return "图表按记录点连线，不要求每天上传"
	}
	return "最近两次上传间隔"
}

// compareMoney 比较两个金额文本。
// 输入：left 和 right 是十进制金额文本。
// 输出：左侧大于、等于或小于右侧时返回 1、0 或 -1。
// 副作用：无。
func compareMoney(left, right string) int {
	// 1. 转换为整数分后比较。
	leftCents, _ := money.ParseCents(left)
	rightCents, _ := money.ParseCents(right)
	if leftCents > rightCents {
		return 1
	}
	if leftCents < rightCents {
		return -1
	}
	return 0
}

// compareDecimal 比较两个普通十进制文本。
// 输入：left 和 right 是小数文本。
// 输出：左侧大于、等于或小于右侧时返回 1、0 或 -1。
// 副作用：无。
func compareDecimal(left, right string) int {
	// 1. 页面百分比精度有限，使用标准浮点解析比较即可。
	leftValue, _ := strconv.ParseFloat(left, 64)
	rightValue, _ := strconv.ParseFloat(right, 64)
	if leftValue > rightValue {
		return 1
	}
	if leftValue < rightValue {
		return -1
	}
	return 0
}
