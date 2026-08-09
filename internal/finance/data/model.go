// Package data 提供 finance 行情数据的 PostgreSQL 仓储和同步服务。
package data

// Daily 描述 PostgreSQL 中的一条股票日线行情。
type Daily struct {
	ID         int64   `json:"id,omitempty"`
	TSCode     string  `json:"ts_code"`
	TradeDate  string  `json:"trade_date"`
	Open       float64 `json:"open"`
	High       float64 `json:"high"`
	Low        float64 `json:"low"`
	Close      float64 `json:"close"`
	PreClose   float64 `json:"pre_close"`
	Change     float64 `json:"change"`
	PctChange  float64 `json:"pct_chg"`
	Volume     float64 `json:"vol"`
	Amount     float64 `json:"amount"`
	CreateDate string  `json:"create_date,omitempty"`
	UpdateDate string  `json:"update_date,omitempty"`
}
