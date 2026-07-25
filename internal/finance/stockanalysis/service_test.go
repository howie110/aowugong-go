package stockanalysis

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/finance/position"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServiceBuildsPortfolioReportAndTreatsStandardBondAsCash 验证组合聚合与现金等价物口径。
// 输入：两日两个账户的资产快照，最新日包含“标准券”持仓。
// 输出：报告正确聚合账户，并把标准券市值从股票仓位移入现金。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestServiceBuildsPortfolioReportAndTreatsStandardBondAsCash(t *testing.T) {
	// 1. 创建完整迁移数据库并写入四条仓位快照。
	ctx := context.Background()
	db := testdatabase.Open(t)
	positionRepository := position.NewRepository(db)
	seedStockAnalysisSnapshot(t, ctx, positionRepository, "2026-07-14", "5042", 100000, 60000, 40000, nil)
	seedStockAnalysisSnapshot(t, ctx, positionRepository, "2026-07-14", "7521", 200000, 120000, 80000, nil)
	seedStockAnalysisSnapshot(t, ctx, positionRepository, "2026-07-15", "5042", 110000, 70000, 40000, []position.Holding{
		{SecurityName: "贵州茅台", MarketValue: 50000},
		{SecurityName: "标准券", MarketValue: 20000},
	})
	seedStockAnalysisSnapshot(t, ctx, positionRepository, "2026-07-15", "7521", 220000, 130000, 90000, []position.Holding{
		{SecurityName: "贵州茅台", MarketValue: 130000},
	})

	// 2. 生成统一报告和页面摘要。
	service := NewService(NewRepository(db))
	report, err := service.Report(ctx, 500)
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	summary, err := service.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	// 3. 核对最新组合、变化、账户和持仓分布。
	if report.Latest == nil || report.Latest.TotalAsset != "330000.00" {
		t.Fatalf("latest = %#v, want total asset 330000.00", report.Latest)
	}
	if report.Latest.MarketValue != "180000.00" || report.Latest.AvailableCash != "150000.00" {
		t.Errorf("latest market/cash = %s/%s, want 180000.00/150000.00", report.Latest.MarketValue, report.Latest.AvailableCash)
	}
	if report.Changes.TotalAssetChange != "30000.00" || len(report.Accounts) != 2 {
		t.Errorf("changes/accounts = %#v/%d", report.Changes, len(report.Accounts))
	}
	if len(report.Holdings) != 2 || report.Holdings[0].SecurityName != "贵州茅台" || report.Holdings[1].SecurityName != "现金" {
		t.Errorf("holdings = %#v, want stock and cash only", report.Holdings)
	}
	if report.Holdings[0].Accounts != "账户-5042 / 账户-7521" {
		t.Errorf("holding accounts = %q, want account suffix order", report.Holdings[0].Accounts)
	}
	if summary.Metrics[0].Value != "330,000.00" || summary.Metrics[3].Value != "2" {
		t.Errorf("summary metrics = %#v", summary.Metrics)
	}
}

// seedStockAnalysisSnapshot 写入仓位分析测试使用的单账户快照。
// 输入：日期、账户、资产金额和可选持仓明细。
// 输出：无，写入失败时终止测试。
// 副作用：写入测试 SQLite 的资产和持仓表。
func seedStockAnalysisSnapshot(t *testing.T, ctx context.Context, repository *position.Repository, date, suffix string, total, market, cash float64, holdings []position.Holding) {
	// 1. 组装账户快照并仅在提供明细时标记解析成功。
	t.Helper()
	snapshot := position.Snapshot{
		SnapshotDate: date, BrokerName: "东莞证券", SourceApp: "同花顺",
		AccountSuffix: suffix, AccountAlias: "账户-" + suffix,
		TotalAsset: total, MarketValue: market, AvailableCash: cash,
		Holdings: holdings, HoldingsParsed: holdings != nil,
	}

	// 2. 通过正式仓储入口写入样本。
	if _, err := repository.Upsert(ctx, snapshot, map[string]any{}, "test"); err != nil {
		t.Fatalf("Upsert(%s, %s) error = %v", date, suffix, err)
	}
}
