package backtest

import (
	"math"
	"time"
)

// buildDrawdowns 计算累计收益、历史高点和逐周期回撤。
// 输入：details 是回测明细，positionSize 是交易单位。
// 输出：返回与明细等长的回撤序列。
// 副作用：无。
func buildDrawdowns(details []Detail, positionSize float64) []DrawdownDetail {
	// 1. 逐项累计收益，并沿用旧项目忽略零值高点的规则。
	result := make([]DrawdownDetail, 0, len(details))
	cumulative := float64(0)
	maximum := float64(0)
	hasMaximum := false
	for _, detail := range details {
		cumulative += detail.Profit
		if cumulative != 0 && (!hasMaximum || cumulative > maximum) {
			maximum = cumulative
			hasMaximum = true
		}
		drawdown := cumulative - maximum
		drawdownRate := float64(0)
		if maximum > 0 {
			drawdownRate = safeDivide(drawdown, detail.Close*positionSize)
		}
		result = append(result, DrawdownDetail{
			Detail: detail, CumulativeProfit: cumulative, CumulativeProfitMax: maximum,
			Drawdown: drawdown, DrawdownRate: drawdownRate,
		})
	}
	return result
}

// buildTrades 按有效交易编号聚合毛收益、费用、净收益和持有周期。
// 输入：details 是逐周期回测明细。
// 输出：返回按交易编号升序排列的统计，不包含未持仓的零号周期。
// 副作用：无。
func buildTrades(details []Detail) []TradeSummary {
	// 1. 利用行情已排序和交易编号递增的特性顺序聚合。
	result := make([]TradeSummary, 0)
	indexes := make(map[int]int)
	for _, detail := range details {
		if detail.TradeNum == 0 {
			continue
		}
		index, exists := indexes[detail.TradeNum]
		if !exists {
			index = len(result)
			indexes[detail.TradeNum] = index
			result = append(result, TradeSummary{TradeNum: detail.TradeNum})
		}
		result[index].GrossProfit += detail.GrossProfit
		result[index].Commission += detail.Commission
		result[index].Profit += detail.Profit
		result[index].PeriodCount++
	}
	return result
}

// buildAssessment 汇总交易收益、费用、波动、回撤和年化指标。
// 输入：drawdowns 是回撤明细，trades 是按笔统计，positionSize 是交易单位。
// 输出：返回整体评估；没有成交时交易指标保持零值。
// 副作用：无。
func buildAssessment(drawdowns []DrawdownDetail, trades []TradeSummary, positionSize float64) Assessment {
	// 1. 初始化价格、时间和回撤边界。
	assessment := Assessment{}
	if len(drawdowns) == 0 {
		return assessment
	}
	assessment.StartTime = drawdowns[0].Time
	assessment.EndTime = drawdowns[len(drawdowns)-1].Time
	assessment.DateCount = dateCount(assessment.StartTime, assessment.EndTime)
	assessment.MaximumPrice = drawdowns[0].Close
	assessment.MaximumDrawdown = drawdowns[0].Drawdown
	assessment.MaximumDrawdownRate = drawdowns[0].DrawdownRate
	for _, row := range drawdowns {
		assessment.MaximumPrice = math.Max(assessment.MaximumPrice, row.Close)
		assessment.MaximumDrawdown = math.Min(assessment.MaximumDrawdown, row.Drawdown)
		assessment.MaximumDrawdownRate = math.Min(assessment.MaximumDrawdownRate, row.DrawdownRate)
	}

	// 2. 汇总交易笔数、正负收益、费用和极值。
	assessment.TradeCount = len(trades)
	positiveSum := float64(0)
	negativeSum := float64(0)
	if len(trades) > 0 {
		assessment.MaximumTradeProfit = trades[0].Profit
		assessment.MinimumTradeProfit = trades[0].Profit
	}
	for _, trade := range trades {
		assessment.GrossProfitSum += trade.GrossProfit
		assessment.CommissionSum += trade.Commission
		assessment.ProfitSum += trade.Profit
		assessment.MaximumTradeProfit = math.Max(assessment.MaximumTradeProfit, trade.Profit)
		assessment.MinimumTradeProfit = math.Min(assessment.MinimumTradeProfit, trade.Profit)
		if trade.Profit > 0 {
			assessment.PositiveTradeCount++
			positiveSum += trade.Profit
		} else if trade.Profit < 0 {
			assessment.NegativeTradeCount++
			negativeSum += trade.Profit
		}
	}
	assessment.PositiveRate = safeDivide(float64(assessment.PositiveTradeCount), float64(assessment.TradeCount))
	assessment.CommissionMean = safeDivide(assessment.CommissionSum, float64(assessment.TradeCount))
	assessment.ProfitMean = safeDivide(assessment.ProfitSum, float64(assessment.TradeCount))
	assessment.PositiveProfitMean = safeDivide(positiveSum, float64(assessment.PositiveTradeCount))
	assessment.NegativeProfitMean = safeDivide(negativeSum, float64(assessment.NegativeTradeCount))
	assessment.ProfitMeanRate = safeDivide(assessment.PositiveProfitMean, math.Abs(assessment.NegativeProfitMean))

	// 3. 计算按笔收益总体标准差及资金占用收益率。
	variance := float64(0)
	for _, trade := range trades {
		difference := trade.Profit - assessment.ProfitMean
		variance += difference * difference
	}
	assessment.ProfitStandardDeviation = math.Sqrt(safeDivide(variance, float64(assessment.TradeCount)))
	assessment.ProfitDeviationRate = safeDivide(assessment.ProfitStandardDeviation, assessment.ProfitMean)
	capital := assessment.MaximumPrice * positionSize
	assessment.CommissionRate = safeDivide(assessment.CommissionSum, capital)
	assessment.ReturnRate = safeDivide(assessment.ProfitSum, capital)
	assessment.AnnualReturnRate = safeDivide(assessment.ReturnRate, float64(assessment.DateCount)) * 365
	return assessment
}

// safeDivide 执行分母为零时返回零的浮点除法。
// 输入：numerator 是分子，denominator 是分母。
// 输出：分母非零时返回商，否则返回零。
// 副作用：无。
func safeDivide(numerator, denominator float64) float64 {
	// 1. 拒绝零分母，避免结果出现无穷或非数值。
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

// dateCount 计算回测起止时间之间的自然日数且至少返回一天。
// 输入：start 和 end 是 YYYY-MM-DD、YYYYMMDD 或 RFC3339 时间。
// 输出：可解析时返回自然日间隔，否则返回一天。
// 副作用：无。
func dateCount(start, end string) int {
	// 1. 尝试解析旧项目可能产生的三类时间格式。
	formats := []string{"2006-01-02", "20060102", time.RFC3339}
	for _, format := range formats {
		startTime, startErr := time.Parse(format, start)
		endTime, endErr := time.Parse(format, end)
		if startErr == nil && endErr == nil {
			days := int(endTime.Sub(startTime).Hours() / 24)
			if days > 0 {
				return days
			}
			return 1
		}
	}
	return 1
}
