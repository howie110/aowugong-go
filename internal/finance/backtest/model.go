package backtest

// Market 表示回测使用的交易市场及其计费规则。
type Market string

const (
	// MarketStock 表示 A 股股票市场。
	MarketStock Market = "stock"
	// MarketETF 表示场内 ETF 市场。
	MarketETF Market = "etf"
	// MarketWeb3 表示 Web3 现货市场。
	MarketWeb3 Market = "web3"
)

// Bar 描述一根已经生成策略信号的行情。
type Bar struct {
	Time     string  `json:"time"`
	Open     float64 `json:"open"`
	Close    float64 `json:"close"`
	PreClose float64 `json:"pre_close,omitempty"`
	Enter    bool    `json:"if_in"`
	Exit     bool    `json:"if_out"`
}

// Request 描述纯计算回测所需的行情与可选交易单位。
type Request struct {
	Bars        []Bar   `json:"bars"`
	OperateSize float64 `json:"operate_size,omitempty"`
}

// Detail 描述每根行情完成信号执行后的持仓与收益。
type Detail struct {
	Bar
	OperateNum     float64 `json:"operate_num"`
	PositionBefore float64 `json:"position_before"`
	PositionAfter  float64 `json:"position_after"`
	Commission     float64 `json:"commission"`
	GrossProfit    float64 `json:"gross_profit"`
	Profit         float64 `json:"profit"`
	TradeNum       int     `json:"trade_num"`
}

// DrawdownDetail 描述每根行情的累计收益、高点和回撤。
type DrawdownDetail struct {
	Detail
	CumulativeProfit    float64 `json:"cumulative_profit"`
	CumulativeProfitMax float64 `json:"cumulative_profit_max"`
	Drawdown            float64 `json:"drawdown"`
	DrawdownRate        float64 `json:"drawdown_rate"`
}

// TradeSummary 描述一笔从买入到卖出或回测结束的聚合结果。
type TradeSummary struct {
	TradeNum    int     `json:"trade_num"`
	GrossProfit float64 `json:"gross_profit"`
	Commission  float64 `json:"commission"`
	Profit      float64 `json:"profit"`
	PeriodCount int     `json:"period_count"`
}

// Assessment 描述一次回测的整体收益、风险和时间指标。
type Assessment struct {
	TradeCount              int     `json:"trade_num_max"`
	PositiveTradeCount      int     `json:"trade_num_positive"`
	NegativeTradeCount      int     `json:"trade_num_negative"`
	PositiveRate            float64 `json:"positive_rate"`
	GrossProfitSum          float64 `json:"gross_profit_sum"`
	CommissionSum           float64 `json:"commission_sum"`
	CommissionMean          float64 `json:"commission_mean"`
	CommissionRate          float64 `json:"commission_rate"`
	ProfitSum               float64 `json:"profit_sum"`
	ProfitMean              float64 `json:"profit_mean"`
	PositiveProfitMean      float64 `json:"profit_mean_positive"`
	NegativeProfitMean      float64 `json:"profit_mean_negative"`
	ProfitMeanRate          float64 `json:"profit_mean_rate"`
	ProfitStandardDeviation float64 `json:"profit_std"`
	ProfitDeviationRate     float64 `json:"profit_std_rate"`
	MaximumDrawdown         float64 `json:"drawdown_max"`
	MaximumDrawdownRate     float64 `json:"drawdown_max_rate"`
	MaximumTradeProfit      float64 `json:"profit_max"`
	MinimumTradeProfit      float64 `json:"profit_min"`
	MaximumPrice            float64 `json:"price_max"`
	StartTime               string  `json:"start_time"`
	EndTime                 string  `json:"end_time"`
	DateCount               int     `json:"date_num"`
	ReturnRate              float64 `json:"return_rate"`
	AnnualReturnRate        float64 `json:"return_rate_yearly"`
}

// Result 汇总逐周期明细、回撤、按笔统计和整体评估。
type Result struct {
	Details    []Detail         `json:"details"`
	Drawdowns  []DrawdownDetail `json:"drawdowns"`
	Trades     []TradeSummary   `json:"trades"`
	Assessment Assessment       `json:"assessment"`
}
