package stockanalysis

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/howiedata/aowugong-go/internal/money"
)

type snapshotRow struct {
	SnapshotDate        string
	BrokerName          string
	AccountSuffix       string
	AccountAlias        string
	TotalAssetCents     int64
	MarketValueCents    int64
	AvailableCashCents  int64
	OtherAmountCents    int64
	CashEquivalentCents int64
}

type holdingRow struct {
	SecurityName string
	MarketCents  int64
	Quantity     *string
	AccountCount int
	Accounts     string
}

// Repository 负责股票仓位分析所需的受限 SQLite 查询。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建股票仓位分析仓储。
// 输入：db 是已经迁移的 SQLite 连接。
// 输出：返回只读分析仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存数据库依赖。
	return &Repository{db: db}
}

// snapshots 读取最近资产快照及其中的现金等价物金额。
// 输入：ctx 控制查询，limit 是 1 到 2000 的快照上限。
// 输出：按日期和账户正序返回内部行；失败时返回错误。
// 副作用：只读 SQLite。
func (r *Repository) snapshots(ctx context.Context, limit int) ([]snapshotRow, error) {
	// 1. 限制大表读取范围。
	if limit < 1 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT snapshot_date, broker_name, account_suffix, account_alias,
		       total_asset, market_value, available_cash, other_amount, cash_equivalent_value
		FROM (
			SELECT snapshot.snapshot_date,
			       snapshot.broker_name,
			       snapshot.account_suffix,
			       COALESCE(snapshot.account_alias, '') AS account_alias,
			       snapshot.total_asset,
			       snapshot.market_value,
			       snapshot.available_cash,
			       snapshot.other_amount,
			       COALESCE(cash_equivalent.market_value, 0) AS cash_equivalent_value
			FROM finance_asset_snapshot snapshot
			LEFT JOIN (
				SELECT snapshot_date, account_suffix, SUM(market_value) AS market_value
				FROM finance_position_holding_snapshot
				WHERE security_name = '标准券' AND market_value > 0
				GROUP BY snapshot_date, account_suffix
			) cash_equivalent
			  ON cash_equivalent.snapshot_date = snapshot.snapshot_date
			 AND cash_equivalent.account_suffix = snapshot.account_suffix
			WHERE snapshot.parse_status = 'parsed'
			ORDER BY snapshot.snapshot_date DESC, snapshot.account_suffix ASC
			LIMIT ?
		) recent
		ORDER BY snapshot_date ASC, account_suffix ASC
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询仓位分析快照: %w", err)
	}
	defer rows.Close()

	// 2. 使用统一金额转换把数据库 NUMERIC 转成整数分。
	results := make([]snapshotRow, 0)
	for rows.Next() {
		var item snapshotRow
		var total, market, cash, other, cashEquivalent string
		if err := rows.Scan(
			&item.SnapshotDate, &item.BrokerName, &item.AccountSuffix, &item.AccountAlias,
			&total, &market, &cash, &other, &cashEquivalent,
		); err != nil {
			return nil, fmt.Errorf("扫描仓位分析快照: %w", err)
		}
		values := []*int64{&item.TotalAssetCents, &item.MarketValueCents, &item.AvailableCashCents, &item.OtherAmountCents, &item.CashEquivalentCents}
		texts := []string{total, market, cash, other, cashEquivalent}
		for index := range values {
			parsed, err := money.ParseCents(texts[index])
			if err != nil {
				return nil, fmt.Errorf("转换仓位金额 %q: %w", texts[index], err)
			}
			*values[index] = parsed
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历仓位分析快照: %w", err)
	}
	return results, nil
}

// holdings 读取指定日期按证券聚合的正市值持仓。
// 输入：ctx 控制查询，snapshotDate 是 ISO 日期。
// 输出：按市值倒序返回聚合持仓；失败时返回错误。
// 副作用：只读 SQLite。
func (r *Repository) holdings(ctx context.Context, snapshotDate string) ([]holdingRow, error) {
	// 1. 按日期限制查询并在 SQLite 内完成账户聚合。
	rows, err := r.db.QueryContext(ctx, `
		SELECT security_name,
		       SUM(market_value),
		       CASE WHEN COUNT(quantity) > 0 THEN SUM(quantity) END,
		       COUNT(DISTINCT account_suffix),
		       REPLACE(GROUP_CONCAT(DISTINCT COALESCE(account_alias, account_suffix)), ',', ' / ')
		FROM finance_position_holding_snapshot
		WHERE snapshot_date = ? AND market_value > 0
		GROUP BY security_name
		ORDER BY SUM(market_value) DESC
	`, snapshotDate)
	if err != nil {
		return nil, fmt.Errorf("查询最新持仓分布: %w", err)
	}
	defer rows.Close()

	// 2. 扫描金额与可空数量。
	results := make([]holdingRow, 0)
	for rows.Next() {
		var item holdingRow
		var market string
		var quantity, accounts sql.NullString
		if err := rows.Scan(&item.SecurityName, &market, &quantity, &item.AccountCount, &accounts); err != nil {
			return nil, fmt.Errorf("扫描最新持仓分布: %w", err)
		}
		item.MarketCents, err = money.ParseCents(market)
		if err != nil {
			return nil, fmt.Errorf("转换持仓市值 %q: %w", market, err)
		}
		if quantity.Valid {
			item.Quantity = &quantity.String
		}
		if accounts.Valid {
			item.Accounts = accounts.String
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最新持仓分布: %w", err)
	}
	return results, nil
}
