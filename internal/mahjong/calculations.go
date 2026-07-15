package mahjong

import (
	"time"

	"github.com/howiedata/aowugong-go/internal/money"
)

// buildSummary 计算麻将战绩摘要。
// 输入：records 是日期正序记录，tableFeeCents 是每场费用分。
// 输出：返回完整摘要。
// 副作用：无。
func buildSummary(records []storedRecord, tableFeeCents int64) Summary {
	// 1. 空数据返回页面可直接渲染的全零结构。
	if len(records) == 0 {
		return Summary{
			WinRate:               "0.00",
			TotalResult:           "0.00",
			AverageResult:         "0.00",
			TableFee:              money.FormatCents(tableFeeCents, false),
			AdjustedAverageResult: "0.00",
			SpanYears:             "0.00",
			BestDay:               ExtremeDay{ResultAmount: "0.00"},
			WorstDay:              ExtremeDay{ResultAmount: "0.00"},
			CurrentStreakType:     "none",
		}
	}

	// 2. 聚合胜负、金额、极值和日期跨度。
	winCount := 0
	lossCount := 0
	totalCents := int64(0)
	best := records[0]
	worst := records[0]
	for _, record := range records {
		totalCents += record.amountCents
		if record.amountCents > 0 {
			winCount++
		} else if record.amountCents < 0 {
			lossCount++
		}
		if record.amountCents > best.amountCents {
			best = record
		}
		if record.amountCents < worst.amountCents {
			worst = record
		}
	}
	firstDate, _ := time.Parse(dateLayout, records[0].PlayedDate)
	latestDate, _ := time.Parse(dateLayout, records[len(records)-1].PlayedDate)
	spanDays := int(latestDate.Sub(firstDate).Hours() / 24)
	averageCents := roundedDivide(totalCents, int64(len(records)))
	streak := buildStreak(records)

	// 3. 格式化与旧 Pydantic Decimal JSON 一致的两位小数字符串。
	first := records[0].PlayedDate
	latest := records[len(records)-1].PlayedDate
	bestDate := best.PlayedDate
	worstDate := worst.PlayedDate
	return Summary{
		TotalGames:            len(records),
		WinGames:              winCount,
		LossGames:             lossCount,
		DrawGames:             len(records) - winCount - lossCount,
		WinRate:               formatHundredths(roundedDivide(int64(winCount)*10000, int64(len(records)))),
		TotalResult:           money.FormatCents(totalCents, false),
		AverageResult:         money.FormatCents(averageCents, false),
		TableFee:              money.FormatCents(tableFeeCents, false),
		AdjustedAverageResult: money.FormatCents(averageCents+tableFeeCents, false),
		FirstDate:             &first,
		LatestDate:            &latest,
		SpanDays:              spanDays,
		SpanYears:             formatHundredths(roundedDivide(int64(spanDays)*100, 365)),
		BestDay:               ExtremeDay{PlayedDate: &bestDate, ResultAmount: money.FormatCents(best.amountCents, false)},
		WorstDay:              ExtremeDay{PlayedDate: &worstDate, ResultAmount: money.FormatCents(worst.amountCents, false)},
		CurrentStreakType:     streak.currentType,
		CurrentStreakCount:    streak.currentCount,
		LongestWinStreak:      streak.longestWin,
		LongestLossStreak:     streak.longestLoss,
	}
}

// buildTimeline 构建累计输赢和滚动场均。
// 输入：records 是日期正序记录。
// 输出：返回趋势点。
// 副作用：无。
func buildTimeline(records []storedRecord) []TimelinePoint {
	// 1. 逐场累加并计算截至当前的四舍五入场均。
	points := make([]TimelinePoint, 0, len(records))
	cumulative := int64(0)
	for index, record := range records {
		cumulative += record.amountCents
		points = append(points, TimelinePoint{
			Sequence:         index + 1,
			PlayedDate:       record.PlayedDate,
			ResultAmount:     money.FormatCents(record.amountCents, false),
			CumulativeResult: money.FormatCents(cumulative, false),
			RunningAverage:   money.FormatCents(roundedDivide(cumulative, int64(index+1)), false),
		})
	}
	return points
}

// buildPeriods 按月份或年份聚合麻将战绩。
// 输入：records 是日期正序记录，layout 是 Go 日期格式。
// 输出：返回周期正序统计。
// 副作用：无。
func buildPeriods(records []storedRecord, layout string) []PeriodSummary {
	// 1. 按格式化日期分组并保存稳定周期顺序。
	type periodData struct {
		count int
		wins  int
		loss  int
		total int64
	}
	groups := make(map[string]*periodData)
	periods := make([]string, 0)
	for _, record := range records {
		parsed, _ := time.Parse(dateLayout, record.PlayedDate)
		period := parsed.Format(layout)
		data, exists := groups[period]
		if !exists {
			data = &periodData{}
			groups[period] = data
			periods = append(periods, period)
		}
		data.count++
		data.total += record.amountCents
		if record.amountCents > 0 {
			data.wins++
		} else if record.amountCents < 0 {
			data.loss++
		}
	}

	// 2. 将每个周期转换为页面结构。
	result := make([]PeriodSummary, 0, len(periods))
	for _, period := range periods {
		data := groups[period]
		result = append(result, PeriodSummary{
			Period:        period,
			GameCount:     data.count,
			WinCount:      data.wins,
			LossCount:     data.loss,
			WinRate:       formatHundredths(roundedDivide(int64(data.wins)*10000, int64(data.count))),
			TotalResult:   money.FormatCents(data.total, false),
			AverageResult: money.FormatCents(roundedDivide(data.total, int64(data.count)), false),
		})
	}
	return result
}

// buildFrequency 计算打牌频率、日期间隔和星期分布。
// 输入：records 是日期正序记录。
// 输出：返回频率统计。
// 副作用：无。
func buildFrequency(records []storedRecord) FrequencyStats {
	// 1. 空数据返回稳定的零值与空数组。
	if len(records) == 0 {
		return FrequencyStats{
			AverageGamesPerMonth:    "0.00",
			AverageDaysBetweenGames: "0.00",
			LongestGap:              Gap{},
			WeekdayDistribution:     []WeekdayFrequency{},
		}
	}

	// 2. 计算相邻日期间隔、最近窗口和月份频率。
	totalGapDays := 0
	longestGap := Gap{}
	parsedDates := make([]time.Time, len(records))
	for index, record := range records {
		parsedDates[index], _ = time.Parse(dateLayout, record.PlayedDate)
		if index == 0 {
			continue
		}
		days := int(parsedDates[index].Sub(parsedDates[index-1]).Hours() / 24)
		totalGapDays += days
		if days > longestGap.Days {
			start := records[index-1].PlayedDate
			end := record.PlayedDate
			longestGap = Gap{StartDate: &start, EndDate: &end, Days: days}
		}
	}
	monthly := buildPeriods(records, "2006-01")
	mostActive := monthly[0]
	for _, period := range monthly[1:] {
		if period.GameCount > mostActive.GameCount {
			mostActive = period
		}
	}
	latestDate := parsedDates[len(parsedDates)-1]
	recent90Start := latestDate.AddDate(0, 0, -89)
	recent365Start := latestDate.AddDate(0, 0, -364)

	// 3. 聚合星期分布并选出最常打牌星期。
	weekdayLabels := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}
	weekdayCounts := make([]int, 7)
	recent90 := 0
	recent365 := 0
	for _, date := range parsedDates {
		weekdayIndex := (int(date.Weekday()) + 6) % 7
		weekdayCounts[weekdayIndex]++
		if !date.Before(recent90Start) {
			recent90++
		}
		if !date.Before(recent365Start) {
			recent365++
		}
	}
	favoriteIndex := 0
	distribution := make([]WeekdayFrequency, 0, 7)
	for index, count := range weekdayCounts {
		if count > weekdayCounts[favoriteIndex] {
			favoriteIndex = index
		}
		distribution = append(distribution, WeekdayFrequency{
			Weekday:       index,
			Label:         weekdayLabels[index],
			GameCount:     count,
			WeightPercent: formatHundredths(roundedDivide(int64(count)*10000, int64(len(records)))),
		})
	}

	// 4. 格式化月均和平均间隔。
	mostActiveMonth := mostActive.Period
	favoriteWeekday := weekdayLabels[favoriteIndex]
	averageGapHundredths := int64(0)
	if len(records) > 1 {
		averageGapHundredths = roundedDivide(int64(totalGapDays)*100, int64(len(records)-1))
	}
	return FrequencyStats{
		ActiveMonths:            len(monthly),
		AverageGamesPerMonth:    formatHundredths(roundedDivide(int64(len(records))*100, int64(len(monthly)))),
		AverageDaysBetweenGames: formatHundredths(averageGapHundredths),
		Recent90DayGames:        recent90,
		Recent365DayGames:       recent365,
		MostActiveMonth:         &mostActiveMonth,
		MostActiveMonthGames:    mostActive.GameCount,
		FavoriteWeekday:         &favoriteWeekday,
		FavoriteWeekdayGames:    weekdayCounts[favoriteIndex],
		LongestGap:              longestGap,
		WeekdayDistribution:     distribution,
	}
}

type streakResult struct {
	currentType  string
	currentCount int
	longestWin   int
	longestLoss  int
}

// buildStreak 计算当前和最长连胜连负。
// 输入：records 是日期正序记录。
// 输出：返回连胜连负统计。
// 副作用：无。
func buildStreak(records []storedRecord) streakResult {
	// 1. 平局打断连胜连负，正负结果切换方向。
	result := streakResult{currentType: "none"}
	for _, record := range records {
		nextType := "none"
		if record.amountCents > 0 {
			nextType = "win"
		} else if record.amountCents < 0 {
			nextType = "loss"
		}
		if nextType == "none" {
			result.currentType = "none"
			result.currentCount = 0
		} else if nextType == result.currentType {
			result.currentCount++
		} else {
			result.currentType = nextType
			result.currentCount = 1
		}
		if result.currentType == "win" && result.currentCount > result.longestWin {
			result.longestWin = result.currentCount
		}
		if result.currentType == "loss" && result.currentCount > result.longestLoss {
			result.longestLoss = result.currentCount
		}
	}
	return result
}

// roundedDivide 执行有符号整数的 ROUND_HALF_UP 除法。
// 输入：numerator 是分子，denominator 是正分母。
// 输出：返回四舍五入整数。
// 副作用：无。
func roundedDivide(numerator, denominator int64) int64 {
	// 1. 计算截断商和余数。
	if denominator == 0 {
		return 0
	}
	quotient := numerator / denominator
	remainder := numerator % denominator
	if remainder < 0 {
		remainder = -remainder
	}
	if remainder*2 >= denominator {
		if numerator >= 0 {
			quotient++
		} else {
			quotient--
		}
	}
	return quotient
}

// formatHundredths 把百分之一单位格式化为两位小数。
// 输入：value 是百分之一单位整数。
// 输出：返回两位小数字符串。
// 副作用：无。
func formatHundredths(value int64) string {
	// 1. 百分之一单位与金额分具有相同格式。
	return money.FormatCents(value, false)
}

// publicRecords 提取内部记录中的公开 API 字段。
// 输入：records 是内部记录。
// 输出：返回公开记录列表。
// 副作用：无。
func publicRecords(records []storedRecord) []Record {
	// 1. 按原顺序复制公开字段。
	result := make([]Record, 0, len(records))
	for _, record := range records {
		result = append(result, record.Record)
	}
	return result
}

// signedAmount 给正金额添加加号。
// 输入：value 是两位小数金额文本。
// 输出：返回带符号页面文本。
// 副作用：无。
func signedAmount(value string) string {
	// 1. 使用统一金额解析判断符号。
	cents, err := money.ParseCents(value)
	if err == nil && cents > 0 {
		return "+" + value
	}
	return value
}

// amountStatus 按金额正负返回页面状态色。
// 输入：value 是两位小数金额文本。
// 输出：负值返回 danger，其他返回 normal。
// 副作用：无。
func amountStatus(value string) string {
	// 1. 使用统一金额解析判断状态。
	cents, err := money.ParseCents(value)
	if err == nil && cents < 0 {
		return "danger"
	}
	return "normal"
}
