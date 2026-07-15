package mysqlmigration

// TableSpec 描述已审计有效表及迁移后需要核对的关键范围字段。
type TableSpec struct {
	Name                 string
	RangeColumns         []string
	ColumnRenames        map[string]string
	IgnoredSourceColumns []string
}

// DefaultTableSpecs 返回按外键父表优先排列的二十张当前有效表。
// 输入：无。
// 输出：返回新的迁移规格切片，调用方可安全筛选。
// 副作用：无。
func DefaultTableSpecs() []TableSpec {
	// 1. 顺序保证禁用外键后复制仍便于人工阅读和逐表核验。
	return []TableSpec{
		{Name: "aowugong_fastapi_users", RangeColumns: []string{"id", "created_at"}},
		{Name: "aowugong_roles", RangeColumns: []string{"id", "created_at"}},
		{Name: "aowugong_permissions", RangeColumns: []string{"id", "created_at"}},
		{Name: "aowugong_user_roles", RangeColumns: []string{"user_id", "role_id"}},
		{Name: "aowugong_role_permissions", RangeColumns: []string{"role_id", "permission_id"}},
		{Name: "basic_operation", RangeColumns: []string{"id", "cal_date", "trade_date", "ts_code"}},
		{Name: "basic_position", RangeColumns: []string{"id", "trade_date", "ts_code"}},
		{Name: "finance_broker_account", RangeColumns: []string{"id", "account_suffix"}},
		{Name: "finance_asset_snapshot", RangeColumns: []string{"id", "snapshot_date", "account_suffix"}},
		{Name: "finance_position_holding_snapshot", RangeColumns: []string{"id", "snapshot_date", "account_suffix", "security_code"}},
		{Name: "investment_article_source", RangeColumns: []string{"id", "source_code", "last_fetch_at"}},
		{Name: "investment_article", RangeColumns: []string{"id", "source_id", "published_at"}},
		{Name: "investment_article_analysis", RangeColumns: []string{"id", "article_id", "analyzed_at"}},
		{Name: "mahjong_game_record", RangeColumns: []string{"id", "played_date"}},
		{Name: "service_monitor_result", RangeColumns: []string{"id", "target_code", "checked_at"}},
		{Name: "subscription_record", RangeColumns: []string{"id", "expires_on"}},
		{Name: "tushare_daily", RangeColumns: []string{"id", "ts_code", "trade_date"}},
		{Name: "tushare_etf_basic", RangeColumns: []string{"ts_code", "list_date"}},
		{Name: "tushare_stock_basic", RangeColumns: []string{"ts_code", "list_date"}},
		{Name: "tushare_trade_cal", RangeColumns: []string{"exchange", "cal_date"}},
	}
}

// HistoricalTables 返回六张当前路由、任务和页面不可达的历史表及跳过原因。
// 输入：无。
// 输出：返回新映射，调用方修改不会影响后续盘点。
// 副作用：无。
func HistoricalTables() map[string]string {
	// 1. 明确记录审计结论，未知新表仍会标为 review 而非静默跳过。
	return map[string]string{
		"guestbook":    "当前 API、任务和 React 页面均不可达",
		"revenue":      "当前 API、任务和 React 页面均不可达",
		"user":         "历史示例用户表，当前认证使用 aowugong_fastapi_users",
		"visits":       "历史访问统计，当前页面和任务未读取",
		"vpn_data":     "历史 VPN 数据，当前页面和任务未读取",
		"work_webmaps": "工作导航已迁移为 storage/private/work/navigation.json",
	}
}
