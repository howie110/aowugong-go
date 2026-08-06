package job

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
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

type fakeArticleSyncer struct {
	classifySignals bool
	rebuildCalled   bool
}

// SyncScheduled 返回空生产文章同步摘要。
// 输入：上下文。
// 输出：返回零值成功摘要。
// 副作用：无。
func (s *fakeArticleSyncer) SyncScheduled(_ context.Context, classifySignals bool) (articleanalysis.SyncResult, error) {
	// 1. 记录是否要求完整归类并返回无需处理的测试结果。
	s.classifySignals = classifySignals
	return articleanalysis.SyncResult{}, nil
}

// RebuildSignalGroups 返回固定全局概念组重建摘要。
// 输入：上下文、统计天数和是否应用。
// 输出：返回两个已应用概念组和三个别名。
// 副作用：记录任务是否调用正式应用入口。
func (s *fakeArticleSyncer) RebuildSignalGroups(_ context.Context, _ int, apply bool) (articleanalysis.SignalGroupRebuildResult, error) {
	// 1. 记录调用并返回可格式化的任务摘要。
	s.rebuildCalled = apply
	return articleanalysis.SignalGroupRebuildResult{Applied: apply, GroupCount: 2, AliasCount: 3, PendingAliasCount: 1}, nil
}

// TestSyncInvestmentArticlesSkipsClassificationForManualRun 验证页面手动抓取不等待历史信号归类。
// 输入：携带 manual 来源的统一文章任务。
// 输出：文章服务收到 classifySignals=false。
// 副作用：修改测试替身的 classifySignals 字段。
func TestSyncInvestmentArticlesSkipsClassificationForManualRun(t *testing.T) {
	// 1. 使用记录型文章服务构造任务，并写入手动执行来源。
	articles := &fakeArticleSyncer{classifySignals: true}
	taskSet := &tasks{dependencies: Dependencies{Articles: articles}}
	ctx := scheduler.WithSource(context.Background(), scheduler.SourceManual)

	// 2. 执行任务并核对手动入口没有附带历史归类。
	if _, err := taskSet.syncInvestmentArticles(ctx); err != nil {
		t.Fatalf("syncInvestmentArticles() error = %v", err)
	}
	if articles.classifySignals {
		t.Fatal("manual sync classifySignals = true, want false")
	}
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

// TestRegisterAllAddsEightProductionJobs 验证固定任务名称和频率全部进入统一注册表。
// 输入：完整的测试依赖和空任务注册表。
// 输出：注册七项定时任务和一项仅手动任务，并能通过同一 Run 入口执行测试任务。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestRegisterAllAddsEightProductionJobs(t *testing.T) {
	// 1. 创建完成迁移的 SQLite 和任务注册表。
	ctx := context.Background()
	db := testdatabase.Open(t)
	registry := scheduler.NewRegistry(db, fakeNotification{}, nil)
	dependencies := Dependencies{
		DB: db, Data: fakeDataUpdater{}, Articles: &fakeArticleSyncer{}, Monitoring: fakeMonitor{},
		Subscriptions: fakeSubscription{}, Notification: fakeNotification{}, BackupDir: t.TempDir(),
		BackupRetention: 7, Now: time.Now,
		Backup: func(context.Context, *sql.DB, string, int, time.Time) (string, error) {
			return "test-backup.db", nil
		},
	}

	// 2. 注册并核对八项固定名称、频率和仅手动属性。
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 8 {
		t.Fatalf("definition count = %d, want 8", len(definitions))
	}
	wanted := map[string]string{
		"test_crontab": "0 9 * * *", "update_tushare_daily_data": "",
		"sync_investment_articles": "0 8,20 * * *", "check_service_monitors": "0 22 * * *",
		"check_subscription_expiry_notify": "30 9 * * *", "openilink_reply_reminder": "0 10 * * *",
		"backup_sqlite":                    "30 3 * * *",
		"rebuild_investment_signal_groups": "",
	}
	for _, definition := range definitions {
		if wanted[definition.Name] != definition.Schedule {
			t.Errorf("schedule %s = %q, want %q", definition.Name, definition.Schedule, wanted[definition.Name])
		}
		if (definition.Name == "rebuild_investment_signal_groups" ||
			definition.Name == "update_tushare_daily_data") && !definition.ManualOnly {
			t.Errorf("%s should be manual-only", definition.Name)
		}
	}

	// 3. 通过统一入口执行链路测试任务。
	result, err := registry.Run(ctx, "test_crontab", scheduler.SourceManual)
	if err != nil || result.Status != "success" {
		t.Errorf("Run(test_crontab) result = %+v error = %v", result, err)
	}
}

// TestRebuildInvestmentSignalGroupsAppliesGlobalDictionary 验证手动任务调用全局词典正式应用入口。
// 输入：返回固定摘要的文章服务。
// 输出：任务要求应用六十天重组，并返回组、别名和待归类数量。
// 副作用：修改测试替身的 rebuildCalled 字段。
func TestRebuildInvestmentSignalGroupsAppliesGlobalDictionary(t *testing.T) {
	// 1. 用文章服务替身构造任务并执行。
	articles := &fakeArticleSyncer{}
	taskSet := &tasks{dependencies: Dependencies{Articles: articles}}
	message, err := taskSet.rebuildInvestmentSignalGroups(context.Background())
	if err != nil {
		t.Fatalf("rebuildInvestmentSignalGroups() error = %v", err)
	}

	// 2. 任务必须明确应用词典并返回可读摘要。
	if !articles.rebuildCalled {
		t.Fatal("RebuildSignalGroups() apply = false, want true")
	}
	for _, fragment := range []string{"概念组=2", "别名=3", "待归类=1"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message = %q, missing %q", message, fragment)
		}
	}
}
