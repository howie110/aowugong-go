package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestDashboardServiceBuildsRuntimeSummaries 验证 finance 摘要使用 SQLite 进度和 Go 运行时配置。
// 输入：临时 SQLite 数据与关闭的真实交易配置。
// 输出：各页面摘要包含最新日期、七个内嵌任务和安全交易状态。
// 副作用：在测试临时目录创建 SQLite 文件。
func TestDashboardServiceBuildsRuntimeSummaries(t *testing.T) {
	// 1. 创建包含当前行情日期的最小 SQLite 数据集。
	ctx := context.Background()
	db := openDashboardTestDB(t, ctx)
	defer db.Close()

	// 2. 构造服务并读取数据、任务和交易摘要。
	service := NewDashboardService(db, DashboardOptions{
		HTTPAddress:         "0.0.0.0:2346",
		OpenILinkConfigured: true,
		SchedulerEnabled:    true,
	})
	data, err := service.DataSummary(ctx)
	if err != nil {
		t.Fatalf("DataSummary() error = %v", err)
	}
	jobs := service.JobsSummary()
	trading := service.TradingSummary()

	// 3. 核对前端依赖的稳定响应字段。
	if got := data.Tables[0].Latest; got != "2026-07-15" {
		t.Errorf("trade calendar latest = %q, want 2026-07-15", got)
	}
	if got := data.Tables[1].Latest; got != "2026-07-14" {
		t.Errorf("daily latest = %q, want 2026-07-14", got)
	}
	if got := len(jobs.Jobs); got != 7 {
		t.Errorf("jobs count = %d, want 7", got)
	}
	if got := trading.Guards[0].Value; got != "关闭" {
		t.Errorf("real trade value = %q, want 关闭", got)
	}
	if got := trading.Guards[0].Status; got != "safe" {
		t.Errorf("real trade status = %q, want safe", got)
	}
}

// openDashboardTestDB 创建 finance 摘要测试使用的最小 SQLite 数据库。
// 输入：t 管理测试生命周期，ctx 控制数据库操作。
// 输出：返回已创建核心数据表和样例日期的数据库连接。
// 副作用：在测试临时目录写入 SQLite 文件，失败时终止测试。
func openDashboardTestDB(t *testing.T, ctx context.Context) *sql.DB {
	// 1. 打开测试专用 SQLite 文件。
	t.Helper()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "dashboard.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}

	// 2. 创建摘要查询使用的白名单表并写入代表性日期。
	statements := []string{
		`CREATE TABLE tushare_trade_cal (cal_date TEXT)`,
		`CREATE TABLE tushare_daily (trade_date TEXT)`,
		`CREATE TABLE basic_operation (trade_date TEXT)`,
		`CREATE TABLE tushare_stock_basic (update_date TEXT)`,
		`CREATE TABLE tushare_etf_basic (update_date TEXT)`,
		`INSERT INTO tushare_trade_cal(cal_date) VALUES ('2026-07-15')`,
		`INSERT INTO tushare_daily(trade_date) VALUES ('2026-07-14')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			db.Close()
			t.Fatalf("prepare dashboard database: %v", err)
		}
	}
	return db
}
