package backtest

import "math"

// calculateCommission 计算指定市场单次成交的手续费。
// 输入：market 是市场，size 是带方向数量，price 是成交价。
// 输出：返回基础佣金及卖出侧税费的合计金额。
// 副作用：无。
func calculateCommission(market Market, size, price float64) float64 {
	// 1. 按市场应用现有项目中的费率和最低收费规则。
	switch market {
	case MarketETF:
		return 0
	case MarketWeb3:
		return math.Abs(size) * price * 0.001
	default:
		commission := math.Abs(size) * price * 0.0001
		if commission < 5 {
			commission = 5
		}
		if size < 0 {
			commission += math.Abs(size) * price * 0.001
		}
		return commission
	}
}

// calculateGrossProfit 计算一根行情对应的毛收益。
// 输入：market、操作数量、操作后持仓和开收昨收价格。
// 输出：返回与旧项目一致的单周期持仓损益。
// 副作用：无。
func calculateGrossProfit(market Market, operateNum, positionAfter, open, close, preClose float64) float64 {
	// 1. Web3 没有昨收价，成交和持仓都按本周期开收差计算。
	if market == MarketWeb3 {
		if operateNum != 0 {
			return (close - open) * math.Abs(operateNum)
		}
		if positionAfter > 0 {
			return (close - open) * positionAfter
		}
		return 0
	}

	// 2. 股票和 ETF 买入按今开至今收、卖出按昨收至今开计算。
	if operateNum > 0 {
		return (close - open) * math.Abs(operateNum)
	}
	if operateNum < 0 {
		return (open - preClose) * math.Abs(operateNum)
	}
	if positionAfter > 0 {
		return (close - preClose) * positionAfter
	}
	return 0
}
