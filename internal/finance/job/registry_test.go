package job

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	"github.com/howiedata/aowugong-go/internal/githubbackup"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"github.com/howiedata/aowugong-go/internal/vaultwardenbackup"
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

// CheckWeReadCredential 返回固定有效的微信读书凭据寿命摘要。
// 输入：上下文。
// 输出：返回十二小时有效、已检查一次的摘要。
// 副作用：无。
func (s *fakeArticleSyncer) CheckWeReadCredential(context.Context) (articleanalysis.WeReadCredentialCheckResult, error) {
	// 1. 返回可供任务消息断言的固定结果。
	return articleanalysis.WeReadCredentialCheckResult{
		Status: "valid", ValidFor: "12小时", CheckCount: 1, AccountCount: 7,
	}, nil
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

type fakeGitHubBackuper struct {
	result githubbackup.Result
	err    error
}

type fakeVaultwardenBackupMailer struct {
	result vaultwardenbackup.Result
	err    error
}

// SendLatest 返回固定 Vaultwarden 邮件备份结果。
// 输入：任务上下文。
// 输出：返回测试指定的结果和错误。
// 副作用：无。
func (m fakeVaultwardenBackupMailer) SendLatest(context.Context) (vaultwardenbackup.Result, error) {
	// 1. 返回构造时指定的测试结果。
	return m.result, m.err
}

// Backup 返回固定 GitHub 代码备份摘要。
// 输入：上下文。
// 输出：返回测试配置的结果和错误。
// 副作用：无。
func (b fakeGitHubBackuper) Backup(context.Context) (githubbackup.Result, error) {
	// 1. 返回构造时指定的备份结果。
	return b.result, b.err
}

type captureNotification struct {
	body string
}

// Text 记录 GitHub 代码备份成功通知正文。
// 输入：上下文、标题、正文和接收人。
// 输出：始终返回 nil。
// 副作用：覆盖测试替身保存的正文。
func (n *captureNotification) Text(_ context.Context, _ []string, body, _ string) error {
	// 1. 保存正文供断言。
	n.body = body
	return nil
}

// TestRegisterAllAddsNineProductionJobs 验证固定任务名称和频率全部进入统一注册表。
// 输入：完整的测试依赖和空任务注册表。
// 输出：注册七项定时任务和两项仅手动任务，并能通过同一 Run 入口执行测试任务。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestRegisterAllAddsNineProductionJobs(t *testing.T) {
	// 1. 创建完成迁移的 SQLite 和任务注册表。
	ctx := context.Background()
	db := testdatabase.Open(t)
	registry := scheduler.NewRegistry(db, fakeNotification{}, nil)
	dependencies := Dependencies{
		DB: db, Data: fakeDataUpdater{}, Articles: &fakeArticleSyncer{}, Monitoring: fakeMonitor{},
		Subscriptions: fakeSubscription{}, Notification: fakeNotification{}, BackupDir: t.TempDir(),
		BackupRetention: 7, Now: time.Now,
		Backup: func(context.Context, string, int, time.Time) (string, error) {
			return "test-backup.dump", nil
		},
	}

	// 2. 注册并核对九项固定名称、频率和仅手动属性。
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 9 {
		t.Fatalf("definition count = %d, want 9", len(definitions))
	}
	wanted := map[string]string{
		"test_crontab": "0 9 * * *", "update_tushare_daily_data": "",
		"sync_investment_articles": "0 8,20 * * *", "check_service_monitors": "0 22 * * *",
		"check_weread_credential":          "",
		"check_subscription_expiry_notify": "30 9 * * *", "openilink_reply_reminder": "0 10 * * *",
		"backup_postgres":                  "30 3 * * *",
		"rebuild_investment_signal_groups": "",
	}
	for _, definition := range definitions {
		if wanted[definition.Name] != definition.Schedule {
			t.Errorf("schedule %s = %q, want %q", definition.Name, definition.Schedule, wanted[definition.Name])
		}
		if (definition.Name == "rebuild_investment_signal_groups" ||
			definition.Name == "update_tushare_daily_data" ||
			definition.Name == "check_weread_credential") && !definition.ManualOnly {
			t.Errorf("%s should be manual-only", definition.Name)
		}
	}

	// 3. 通过统一入口执行链路测试任务。
	result, err := registry.Run(ctx, "test_crontab", scheduler.SourceManual)
	if err != nil || result.Status != "success" {
		t.Errorf("Run(test_crontab) result = %+v error = %v", result, err)
	}
}

// TestRegisterAllAddsOptionalGitHubBackupJob 验证启用服务后注册周日代码备份任务。
// 输入：包含 GitHub 备份服务的完整任务依赖。
// 输出：注册 backup_github_code，频率为周日 04:00。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestRegisterAllAddsOptionalGitHubBackupJob(t *testing.T) {
	// 1. 创建注册表并提供包含 GitHub 备份的完整依赖。
	db := testdatabase.Open(t)
	registry := scheduler.NewRegistry(db, fakeNotification{}, nil)
	dependencies := Dependencies{
		DB: db, Data: fakeDataUpdater{}, Articles: &fakeArticleSyncer{}, Monitoring: fakeMonitor{},
		Subscriptions: fakeSubscription{}, Notification: fakeNotification{}, BackupDir: t.TempDir(),
		BackupRetention: 7, Now: time.Now, GitHubBackup: fakeGitHubBackuper{},
		Backup: func(context.Context, string, int, time.Time) (string, error) {
			return "test-backup.dump", nil
		},
	}
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	// 2. 查找可选任务并核对周日四点频率。
	found := false
	for _, definition := range registry.Definitions() {
		if definition.Name == "backup_github_code" {
			found = true
			if definition.Schedule != "0 4 * * 0" {
				t.Errorf("backup_github_code schedule = %q", definition.Schedule)
			}
		}
	}
	if !found || len(registry.Definitions()) != 10 {
		t.Errorf("definitions = %+v", registry.Definitions())
	}
}

// TestRegisterAllAddsOptionalVaultwardenEmailJob 验证启用服务后注册周日密码库邮件任务。
// 输入：包含 Vaultwarden 邮件备份服务的完整任务依赖。
// 输出：注册 email_vaultwarden_backup，频率为周日 05:00。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestRegisterAllAddsOptionalVaultwardenEmailJob(t *testing.T) {
	// 1. 创建注册表并提供密码库邮件备份替身。
	db := testdatabase.Open(t)
	registry := scheduler.NewRegistry(db, fakeNotification{}, nil)
	dependencies := Dependencies{
		DB: db, Data: fakeDataUpdater{}, Articles: &fakeArticleSyncer{}, Monitoring: fakeMonitor{},
		Subscriptions: fakeSubscription{}, Notification: fakeNotification{}, BackupDir: t.TempDir(),
		BackupRetention: 7, Now: time.Now, VaultwardenBackup: fakeVaultwardenBackupMailer{},
		Backup: func(context.Context, string, int, time.Time) (string, error) {
			return "test-backup.dump", nil
		},
	}
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	// 2. 查找可选任务并核对周日五点频率。
	found := false
	for _, definition := range registry.Definitions() {
		if definition.Name == "email_vaultwarden_backup" {
			found = true
			if definition.Schedule != "0 5 * * 0" {
				t.Errorf("email_vaultwarden_backup schedule = %q", definition.Schedule)
			}
		}
	}
	if !found || len(registry.Definitions()) != 10 {
		t.Errorf("definitions = %+v", registry.Definitions())
	}
}

// TestEmailVaultwardenBackupReturnsSummary 验证密码库邮件任务返回可追踪摘要。
// 输入：固定源文件、大小和收件人结果。
// 输出：摘要包含三项关键信息。
// 副作用：无。
func TestEmailVaultwardenBackupReturnsSummary(t *testing.T) {
	// 1. 使用固定服务结果执行任务。
	taskSet := &tasks{dependencies: Dependencies{VaultwardenBackup: fakeVaultwardenBackupMailer{result: vaultwardenbackup.Result{
		SourcePath: "/backup/vaultwarden.tar.gz", Size: 1024, EmailTo: "825360699@qq.com",
	}}}}
	message, err := taskSet.emailVaultwardenBackup(context.Background())
	if err != nil {
		t.Fatalf("emailVaultwardenBackup() error = %v", err)
	}

	// 2. 核对任务摘要包含文件、大小和邮箱。
	for _, fragment := range []string{"vaultwarden.tar.gz", "1024", "825360699@qq.com"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message = %q, missing %q", message, fragment)
		}
	}
}

// TestBackupGitHubCodeSendsSuccessSummary 验证代码备份成功后只发送一条汇总通知。
// 输入：两个仓库均更新成功的结果。
// 输出：返回计数摘要且通知包含四段任务信息。
// 副作用：写入通知测试替身。
func TestBackupGitHubCodeSendsSuccessSummary(t *testing.T) {
	// 1. 使用固定备份结果、时钟和通知替身执行任务。
	notification := &captureNotification{}
	taskSet := &tasks{dependencies: Dependencies{
		GitHubBackup: fakeGitHubBackuper{result: githubbackup.Result{DiscoveredCount: 2, UpdatedCount: 2}},
		Notification: notification,
		Now:          func() time.Time { return time.Date(2026, 8, 9, 4, 0, 0, 0, time.Local) },
	}}
	message, err := taskSet.backupGitHubCode(context.Background())
	if err != nil {
		t.Fatalf("backupGitHubCode() error = %v", err)
	}

	// 2. 核对任务摘要和四段式成功通知。
	for _, fragment := range []string{"仓库=2", "更新=2", "失败=0"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message = %q, missing %q", message, fragment)
		}
	}
	for _, fragment := range []string{"任务：backup_github_code", "时间：2026-08-09 04:00:00", "状态：执行成功", "信息：" + message} {
		if !strings.Contains(notification.body, fragment) {
			t.Errorf("notification = %q, missing %q", notification.body, fragment)
		}
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
