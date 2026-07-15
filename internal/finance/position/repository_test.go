package position

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestRepositoryUpsertsDailyAccountAndReplacesHoldings 验证仓位快照覆盖和明细替换规则。
// 输入：同一日期、同一账户的两次仓位快照。
// 输出：只保留一个最新资产快照，并以第二次持仓明细为准。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestRepositoryUpsertsDailyAccountAndReplacesHoldings(t *testing.T) {
	// 1. 创建完整迁移数据库并同步默认账户别名。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "position.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository := NewRepository(db)
	if err := repository.SyncDefaultAccounts(ctx); err != nil {
		t.Fatalf("SyncDefaultAccounts() error = %v", err)
	}

	// 2. 首次写入资产和一条持仓，再以同日同账户快照覆盖。
	first := Snapshot{
		SnapshotDate: "2026-07-15", BrokerName: "东莞证券", SourceApp: "同花顺",
		AccountSuffix: "5042", AccountAlias: "东莞证券-邓子豪",
		TotalAsset: 100000, MarketValue: 60000, AvailableCash: 40000, OtherAmount: 0,
		HoldingsParsed: true,
		Holdings:       []Holding{{SecurityName: "第一只股票", MarketValue: 60000}},
	}
	if _, err := repository.Upsert(ctx, first, map[string]any{"request": "first"}, "admin"); err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}
	second := first
	second.TotalAsset = 120000
	second.MarketValue = 70000
	second.AvailableCash = 50000
	second.Holdings = []Holding{{SecurityName: "第二只股票", MarketValue: 70000}}
	stored, err := repository.Upsert(ctx, second, map[string]any{"request": "second"}, "admin")
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	// 3. 核对覆盖后的快照、别名回退和唯一持仓明细。
	if stored.TotalAsset != 120000 || stored.ID == 0 {
		t.Errorf("stored = %#v, want updated total asset", stored)
	}
	recent, err := repository.Recent(ctx, 20)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 1 || recent[0].TotalAsset != 120000 {
		t.Errorf("recent = %#v, want one updated snapshot", recent)
	}
	holdings, err := repository.HoldingsByDate(ctx, "2026-07-15")
	if err != nil {
		t.Fatalf("HoldingsByDate() error = %v", err)
	}
	if len(holdings) != 1 || holdings[0].SecurityName != "第二只股票" {
		t.Errorf("holdings = %#v, want replaced holding", holdings)
	}
	alias, err := repository.AccountAlias(ctx, "错误券商", "5042")
	if err != nil {
		t.Fatalf("AccountAlias() error = %v", err)
	}
	if alias != "东莞证券-邓子豪" {
		t.Errorf("alias = %q, want 东莞证券-邓子豪", alias)
	}
}

// TestSyncDefaultAccountsDoesNotRewriteUnchangedRows 验证启动初始化不会污染已迁移账户时间。
// 输入：内容已等于默认值但 updated_at 为历史时间的账户。
// 输出：再次同步后历史更新时间保持不变。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestSyncDefaultAccountsDoesNotRewriteUnchangedRows(t *testing.T) {
	// 1. 创建迁移库、默认账户并设置可识别的历史更新时间。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "account-defaults.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository := NewRepository(db)
	if err := repository.SyncDefaultAccounts(ctx); err != nil {
		t.Fatalf("first SyncDefaultAccounts() error = %v", err)
	}
	const historical = "2026-06-16 13:46:25"
	if _, err := db.ExecContext(ctx, "UPDATE finance_broker_account SET updated_at = ?", historical); err != nil {
		t.Fatalf("set historical updated_at: %v", err)
	}

	// 2. 相同默认值再次同步不能产生 UPDATE。
	if err := repository.SyncDefaultAccounts(ctx); err != nil {
		t.Fatalf("second SyncDefaultAccounts() error = %v", err)
	}
	var updatedAt string
	if err := db.QueryRowContext(ctx, "SELECT updated_at FROM finance_broker_account WHERE account_suffix = '5042'").Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at: %v", err)
	}
	if updatedAt != historical {
		t.Fatalf("updated_at = %q, want %q", updatedAt, historical)
	}
}
