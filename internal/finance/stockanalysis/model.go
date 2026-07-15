// Package stockanalysis 提供仓位快照的组合分析。
package stockanalysis

// Metric 描述股票仓位分析页顶部指标。
type Metric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// Summary 描述股票仓位分析页轻量摘要。
type Summary struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Metrics     []Metric `json:"metrics"`
}

// TimelinePoint 描述一个日期的组合资产聚合点。
type TimelinePoint struct {
	SnapshotDate     string `json:"snapshot_date"`
	TotalAsset       string `json:"total_asset"`
	MarketValue      string `json:"market_value"`
	AvailableCash    string `json:"available_cash"`
	OtherAmount      string `json:"other_amount"`
	PositionPercent  string `json:"position_percent"`
	DailyChange      string `json:"daily_change"`
	CumulativeChange string `json:"cumulative_change"`
	AccountCount     int    `json:"account_count"`
}

// AccountSummary 描述一个账户的最新状态及变化。
type AccountSummary struct {
	AccountSuffix    string `json:"account_suffix"`
	AccountAlias     string `json:"account_alias"`
	BrokerName       string `json:"broker_name"`
	SnapshotDate     string `json:"snapshot_date"`
	TotalAsset       string `json:"total_asset"`
	MarketValue      string `json:"market_value"`
	AvailableCash    string `json:"available_cash"`
	OtherAmount      string `json:"other_amount"`
	PositionPercent  string `json:"position_percent"`
	DailyChange      string `json:"daily_change"`
	CumulativeChange string `json:"cumulative_change"`
}

// HoldingDistribution 描述最新日期的证券或现金资产分布。
type HoldingDistribution struct {
	SecurityName  string  `json:"security_name"`
	MarketValue   string  `json:"market_value"`
	Quantity      *string `json:"quantity"`
	WeightPercent string  `json:"weight_percent"`
	AccountCount  int     `json:"account_count"`
	Accounts      string  `json:"accounts"`
}

// Insight 描述仓位分析提示卡片。
type Insight struct {
	Title  string `json:"title"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
}

// AnalysisIdea 描述后续可扩展的分析方向。
type AnalysisIdea struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Changes 描述首尾和最近记录之间的资产变化。
type Changes struct {
	TotalAssetChange      string `json:"total_asset_change"`
	MarketValueChange     string `json:"market_value_change"`
	AvailableCashChange   string `json:"available_cash_change"`
	DailyTotalAssetChange string `json:"daily_total_asset_change"`
}

// Report 描述股票仓位分析页完整响应。
type Report struct {
	Latest        *TimelinePoint        `json:"latest"`
	First         *TimelinePoint        `json:"first"`
	Previous      *TimelinePoint        `json:"previous"`
	Changes       Changes               `json:"changes"`
	Timeline      []TimelinePoint       `json:"timeline"`
	Accounts      []AccountSummary      `json:"accounts"`
	Holdings      []HoldingDistribution `json:"holdings"`
	Insights      []Insight             `json:"insights"`
	Ideas         []AnalysisIdea        `json:"ideas"`
	SnapshotCount int                   `json:"snapshot_count"`
	DateCount     int                   `json:"date_count"`
}
