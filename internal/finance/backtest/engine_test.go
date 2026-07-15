package backtest

import (
	"context"
	"math"
	"testing"
)

// TestRunStockBacktestDelaysSignalsAndCalculatesFees 验证股票信号延后一周期执行及手续费规则。
// 输入：三根行情，首根发出买入信号，第二根发出卖出信号。
// 输出：第二根买入、第三根卖出，并返回正确收益、交易统计和评估。
// 副作用：无，不访问数据库、不发送通知。
func TestRunStockBacktestDelaysSignalsAndCalculatesFees(t *testing.T) {
	// 1. 构造一笔完整股票交易，信号都应延后一根行情执行。
	request := Request{
		Bars: []Bar{
			{Time: "2026-01-01", Open: 10, Close: 10, PreClose: 10, Enter: true},
			{Time: "2026-01-02", Open: 11, Close: 12, PreClose: 10, Exit: true},
			{Time: "2026-01-03", Open: 13, Close: 14, PreClose: 12},
		},
	}

	// 2. 执行纯计算回测并核对每根行情的操作与净收益。
	result, err := RunStockBacktest(context.Background(), request)
	if err != nil {
		t.Fatalf("RunStockBacktest() error = %v", err)
	}
	if len(result.Details) != 3 {
		t.Fatalf("detail count = %d, want 3", len(result.Details))
	}
	assertFloat(t, "first operation", result.Details[0].OperateNum, 0)
	assertFloat(t, "buy operation", result.Details[1].OperateNum, 100)
	assertFloat(t, "buy commission", result.Details[1].Commission, 5)
	assertFloat(t, "buy profit", result.Details[1].Profit, 95)
	assertFloat(t, "sell operation", result.Details[2].OperateNum, -100)
	assertFloat(t, "sell commission", result.Details[2].Commission, 6.3)
	assertFloat(t, "sell profit", result.Details[2].Profit, 93.7)

	// 3. 核对按笔聚合、累计收益和整体评估。
	if len(result.Trades) != 1 {
		t.Fatalf("trade count = %d, want 1", len(result.Trades))
	}
	assertFloat(t, "trade gross profit", result.Trades[0].GrossProfit, 200)
	assertFloat(t, "trade commission", result.Trades[0].Commission, 11.3)
	assertFloat(t, "trade profit", result.Trades[0].Profit, 188.7)
	if result.Trades[0].PeriodCount != 2 {
		t.Errorf("trade period count = %d, want 2", result.Trades[0].PeriodCount)
	}
	assertFloat(t, "cumulative profit", result.Drawdowns[2].CumulativeProfit, 188.7)
	if result.Assessment.TradeCount != 1 || result.Assessment.PositiveTradeCount != 1 {
		t.Errorf("assessment trades = %+v", result.Assessment)
	}
	assertFloat(t, "positive rate", result.Assessment.PositiveRate, 1)
}

// TestRunMarketBacktestsUseTheirOwnCommission 验证 ETF 与 Web3 使用各自的交易单位和手续费。
// 输入：可触发一次买入的三根简化行情。
// 输出：ETF 按一万份且零手续费，Web3 按一百份且收千分之一手续费。
// 副作用：无，不访问数据库、不发送通知。
func TestRunMarketBacktestsUseTheirOwnCommission(t *testing.T) {
	// 1. 构造 ETF 日线和 Web3 周期线的公共交易场景。
	etfBars := []Bar{
		{Time: "2026-01-01", Open: 1, Close: 1, PreClose: 1, Enter: true},
		{Time: "2026-01-02", Open: 1.1, Close: 1.2, PreClose: 1},
	}
	webBars := []Bar{
		{Time: "2026-01-01T01:00:00+08:00", Open: 10, Close: 10, Enter: true},
		{Time: "2026-01-01T02:00:00+08:00", Open: 11, Close: 12},
	}

	// 2. 分别执行并核对默认交易单位与手续费。
	etf, err := RunETFBacktest(context.Background(), Request{Bars: etfBars})
	if err != nil {
		t.Fatalf("RunETFBacktest() error = %v", err)
	}
	web, err := RunWeb3Backtest(context.Background(), Request{Bars: webBars})
	if err != nil {
		t.Fatalf("RunWeb3Backtest() error = %v", err)
	}
	assertFloat(t, "ETF operation", etf.Details[1].OperateNum, 10000)
	assertFloat(t, "ETF commission", etf.Details[1].Commission, 0)
	assertFloat(t, "Web3 operation", web.Details[1].OperateNum, 100)
	assertFloat(t, "Web3 commission", web.Details[1].Commission, 1.1)
	assertFloat(t, "Web3 gross profit", web.Details[1].GrossProfit, 100)
}

// TestRunStockBacktestRejectsInvalidBars 验证股票回测拒绝缺失昨收价和乱序行情。
// 输入：缺失昨收价或时间倒序的行情。
// 输出：返回带业务上下文的参数错误。
// 副作用：无，不访问数据库、不发送通知。
func TestRunStockBacktestRejectsInvalidBars(t *testing.T) {
	// 1. 准备两个会破坏收益计算确定性的无效请求。
	tests := []struct {
		name string
		bars []Bar
	}{
		{name: "missing pre-close", bars: []Bar{{Time: "2026-01-01", Open: 10, Close: 10}}},
		{name: "unsorted time", bars: []Bar{
			{Time: "2026-01-02", Open: 10, Close: 10, PreClose: 10},
			{Time: "2026-01-01", Open: 10, Close: 10, PreClose: 10},
		}},
	}

	// 2. 逐项执行并确认错误不会被静默吞掉。
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := RunStockBacktest(context.Background(), Request{Bars: test.bars})
			if err == nil {
				t.Fatal("RunStockBacktest() error = nil, want validation error")
			}
		})
	}
}

// assertFloat 比较浮点结果是否在允许误差内。
// 输入：测试对象、字段名、实际值和期望值。
// 输出：无；超出误差时报告测试失败。
// 副作用：可能把失败信息写入测试日志。
func assertFloat(t *testing.T, name string, got, want float64) {
	// 1. 标记辅助函数并按固定精度比较。
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Errorf("%s = %.8f, want %.8f", name, got, want)
	}
}
