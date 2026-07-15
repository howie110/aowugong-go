package backtest

import (
	"context"
	"fmt"
	"strings"
)

const (
	stockOperateSize = 100
	etfOperateSize   = 10000
	web3OperateSize  = 100
)

// RunStockBacktest 执行单个股票回测，返回明细、回撤明细、交易统计和整体评估。
// 输入：ctx 是调用上下文，req 是已生成信号的股票行情和可选交易单位。
// 输出：返回完整回测结果；失败时返回带业务上下文的错误。
// 副作用：无，不访问数据库、不发送通知。
func RunStockBacktest(ctx context.Context, req Request) (Result, error) {
	// 1. 使用股票市场默认交易单位调用唯一回测状态机。
	return run(ctx, MarketStock, stockOperateSize, req)
}

// RunETFBacktest 执行单个 ETF 回测，返回明细、回撤明细、交易统计和整体评估。
// 输入：ctx 是调用上下文，req 是已生成信号的 ETF 行情和可选交易单位。
// 输出：返回完整回测结果；失败时返回带业务上下文的错误。
// 副作用：无，不访问数据库、不发送通知。
func RunETFBacktest(ctx context.Context, req Request) (Result, error) {
	// 1. 使用 ETF 默认交易单位调用唯一回测状态机。
	return run(ctx, MarketETF, etfOperateSize, req)
}

// RunWeb3Backtest 执行单个 Web3 回测，返回明细、回撤明细、交易统计和整体评估。
// 输入：ctx 是调用上下文，req 是已生成信号的 Web3 行情和可选交易单位。
// 输出：返回完整回测结果；失败时返回带业务上下文的错误。
// 副作用：无，不访问数据库、不发送通知。
func RunWeb3Backtest(ctx context.Context, req Request) (Result, error) {
	// 1. 使用 Web3 默认交易单位调用唯一回测状态机。
	return run(ctx, MarketWeb3, web3OperateSize, req)
}

// run 按市场规则执行唯一的逐周期持仓状态机。
// 输入：ctx 控制取消，market 决定收益和费率，defaultSize 是默认单位，req 提供行情。
// 输出：返回完整回测结果；参数或上下文无效时返回错误。
// 副作用：无，不访问数据库、不发送通知。
func run(ctx context.Context, market Market, defaultSize float64, req Request) (Result, error) {
	// 1. 校验行情并确定本次回测交易单位。
	if err := validateRequest(market, req); err != nil {
		return Result{}, fmt.Errorf("校验%s回测参数: %w", market, err)
	}
	operateSize := req.OperateSize
	if operateSize == 0 {
		operateSize = defaultSize
	}

	// 2. 逐根执行上一周期信号，更新持仓、费用和收益。
	details := make([]Detail, 0, len(req.Bars))
	position := float64(0)
	previousEnter := false
	previousExit := false
	tradeNum := 0
	for index, bar := range req.Bars {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("执行第 %d 根行情: %w", index+1, err)
		}
		operateNum := float64(0)
		if previousEnter && position == 0 {
			operateNum = operateSize
		} else if previousExit && position == operateSize {
			operateNum = -operateSize
		}
		commission := float64(0)
		if operateNum != 0 {
			commission = calculateCommission(market, operateNum, bar.Open)
		}
		positionBefore := position
		position += operateNum
		grossProfit := calculateGrossProfit(market, operateNum, position, bar.Open, bar.Close, bar.PreClose)
		if operateNum > 0 {
			tradeNum++
		}
		rowTradeNum := 0
		if operateNum != 0 || position > 0 {
			rowTradeNum = tradeNum
		}
		details = append(details, Detail{
			Bar: bar, OperateNum: operateNum, PositionBefore: positionBefore,
			PositionAfter: position, Commission: commission, GrossProfit: grossProfit,
			Profit: grossProfit - commission, TradeNum: rowTradeNum,
		})
		previousEnter = bar.Enter
		previousExit = bar.Exit
	}

	// 3. 从唯一明细生成回撤、按笔统计和整体评估。
	drawdowns := buildDrawdowns(details, operateSize)
	trades := buildTrades(details)
	assessment := buildAssessment(drawdowns, trades, operateSize)
	return Result{Details: details, Drawdowns: drawdowns, Trades: trades, Assessment: assessment}, nil
}

// validateRequest 校验回测行情完整性、价格和时间顺序。
// 输入：market 决定是否要求昨收价，req 提供行情和交易单位。
// 输出：参数有效时返回 nil，否则返回具体字段错误。
// 副作用：无。
func validateRequest(market Market, req Request) error {
	// 1. 检查市场、行情数量和可选交易单位。
	if market != MarketStock && market != MarketETF && market != MarketWeb3 {
		return fmt.Errorf("未知市场 %q", market)
	}
	if len(req.Bars) == 0 {
		return fmt.Errorf("行情不能为空")
	}
	if req.OperateSize < 0 {
		return fmt.Errorf("交易单位必须大于零")
	}

	// 2. 检查每根行情价格和严格递增时间。
	previousTime := ""
	for index, bar := range req.Bars {
		if strings.TrimSpace(bar.Time) == "" {
			return fmt.Errorf("第 %d 根行情时间为空", index+1)
		}
		if previousTime != "" && bar.Time <= previousTime {
			return fmt.Errorf("第 %d 根行情时间 %q 未严格递增", index+1, bar.Time)
		}
		if bar.Open <= 0 || bar.Close <= 0 {
			return fmt.Errorf("第 %d 根行情开收价格必须大于零", index+1)
		}
		if market != MarketWeb3 && bar.PreClose <= 0 {
			return fmt.Errorf("第 %d 根行情昨收价必须大于零", index+1)
		}
		previousTime = bar.Time
	}
	return nil
}
