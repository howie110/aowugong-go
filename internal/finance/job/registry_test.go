package job

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
)

type fakeDataUpdater struct{}

// UpdateDaily 返回空日线同步摘要。
// 输入：上下文。
// 输出：返回零值成功摘要。
// 副作用：无。
func (fakeDataUpdater) UpdateDaily(context.Context) (financedata.SyncResult, error) {
	// 1. 返回无需补数的测试结果。
	return financedata.SyncResult{}, nil
}

type fakeArticleSyncer struct{}

// SyncScheduled 返回空生产文章同步摘要。
// 输入：上下文。
// 输出：返回零值成功摘要。
// 副作用：无。
func (fakeArticleSyncer) SyncScheduled(context.Context) (articleanalysis.SyncResult, error) {
	// 1. 返回无需处理的测试结果。
	return articleanalysis.SyncResult{}, nil
}

type fakeMonitor struct{}

// CheckAll 返回空监控摘要。
// 输入：上下文。
// 输出：返回零值成功摘要。
// 副作用：无。
func (fakeMonitor) CheckAll(context.Context) (monitoring.CheckResult, error) {
	// 1. 返回无异常的测试结果。
	return monitoring.CheckResult{}, nil
}

type fakeSubscription struct{}

// ListExpiring 返回空到期订阅列表。
// 输入：上下文和提醒天数。
// 输出：返回空列表。
// 副作用：无。
func (fakeSubscription) ListExpiring(context.Context, int) ([]subscription.Record, error) {
	// 1. 返回无需提醒的测试结果。
	return []subscription.Record{}, nil
}

type fakeNotification struct{}

// Text 模拟成功发送统一通知。
// 输入：上下文、标题、正文和接收人。
// 输出：返回 nil。
// 副作用：无。
func (fakeNotification) Text(context.Context, []string, string, string) error {
	// 1. 返回发送成功。
	return nil
}

// TestRegisterAllAddsSevenProductionJobs 验证固定任务名称和频率全部进入统一注册表。
// 输入：完整的测试依赖和空任务注册表。
// 输出：注册七项任务，并能通过同一 Run 入口执行测试任务。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestRegisterAllAddsSevenProductionJobs(t *testing.T) {
	// 1. 创建完成迁移的 SQLite 和任务注册表。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "jobs.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	registry := scheduler.NewRegistry(db, fakeNotification{}, nil)
	dependencies := Dependencies{
		DB: db, Data: fakeDataUpdater{}, Articles: fakeArticleSyncer{}, Monitoring: fakeMonitor{},
		Subscriptions: fakeSubscription{}, Notification: fakeNotification{}, BackupDir: t.TempDir(),
		BackupRetention: 7, Now: time.Now,
		Backup: func(context.Context, *sql.DB, string, int, time.Time) (string, error) {
			return "test-backup.db", nil
		},
	}

	// 2. 注册并核对七项固定名称与频率。
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 7 {
		t.Fatalf("definition count = %d, want 7", len(definitions))
	}
	wanted := map[string]string{
		"test_crontab": "0 9 * * *", "update_tushare_daily_data": "0 20 * * *",
		"sync_investment_articles": "0 8,20 * * *", "check_service_monitors": "30 8 * * *",
		"check_subscription_expiry_notify": "30 9 * * *", "openilink_reply_reminder": "0 10 * * *",
		"backup_sqlite": "30 3 * * *",
	}
	for _, definition := range definitions {
		if wanted[definition.Name] != definition.Schedule {
			t.Errorf("schedule %s = %q, want %q", definition.Name, definition.Schedule, wanted[definition.Name])
		}
	}

	// 3. 通过统一入口执行链路测试任务。
	result, err := registry.Run(ctx, "test_crontab", scheduler.SourceManual)
	if err != nil || result.Status != "success" {
		t.Errorf("Run(test_crontab) result = %+v error = %v", result, err)
	}
}
