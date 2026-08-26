package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestDashboardServiceBuildsRuntimeSummaries 验证 finance 摘要使用 SQLite 进度和 Go 运行时配置。
// 输入：隔离 SQLite 数据与关闭的真实交易配置。
// 输出：各页面摘要包含最新日期、八个任务定义和安全交易状态。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestDashboardServiceBuildsRuntimeSummaries(t *testing.T) {
	// 1. 创建包含当前行情日期的最小 SQLite 数据集。
	ctx := context.Background()
	db := openDashboardTestDB(t, ctx)

	// 2. 构造服务并读取数据、任务和交易摘要。
	service := NewDashboardService(db, DashboardOptions{
		HTTPAddress:        "0.0.0.0:2346",
		WeComBotConfigured: true,
		SchedulerEnabled:   true,
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

// TestJobsSummaryIncludesEnabledBackupTasks 验证任务页只显示已启用的可选备份任务。
// 输入：同时启用 GitHub 和 Vaultwarden 备份的摘要服务。
// 输出：任务清单增加两个周任务。
// 副作用：无。
func TestJobsSummaryIncludesEnabledBackupTasks(t *testing.T) {
	// 1. 构造启用两项备份的服务并读取任务摘要。
	service := NewDashboardService(nil, DashboardOptions{
		SchedulerEnabled: true, GitHubBackupEnabled: true, VaultwardenBackupEnabled: true,
	})
	jobs := service.JobsSummary()

	// 2. 核对任务数量和两个可选任务名称。
	if len(jobs.Jobs) != 9 {
		t.Fatalf("jobs count = %d, want 9", len(jobs.Jobs))
	}
	wanted := map[string]bool{"backup_github_code": false, "email_vaultwarden_backup": false}
	for _, item := range jobs.Jobs {
		if _, ok := wanted[item.Name]; ok {
			wanted[item.Name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("jobs missing %s", name)
		}
	}
}

// openDashboardTestDB 创建 finance 摘要测试使用的最小 SQLite 数据库。
// 输入：t 管理测试生命周期，ctx 控制数据库操作。
// 输出：返回已创建核心数据表和样例日期的数据库连接。
// 副作用：创建并写入隔离 SQLite 测试库，失败时终止测试。
func openDashboardTestDB(t *testing.T, ctx context.Context) *sql.DB {
	// 1. 打开已完成正式迁移的隔离 SQLite。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 向摘要查询白名单表写入代表性日期。
	statements := []string{
		`INSERT INTO tushare_trade_cal(cal_date) VALUES ('2026-07-15')`,
		`INSERT INTO tushare_daily(trade_date) VALUES ('2026-07-14')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare dashboard database: %v", err)
		}
	}
	return db
}
