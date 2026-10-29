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
	return articleanalysis.SignalGroupRebuildResult{Applied: apply, GroupCount: 2, AliasCount: 3}, nil
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

type fakeMonitor struct {
	result monitoring.CheckResult
	err    error
}

// CheckAll 返回测试指定的监控摘要。
// 输入：上下文。
// 输出：返回构造时指定的结果和错误。
// 副作用：无。
func (m fakeMonitor) CheckAll(context.Context) (monitoring.CheckResult, error) {
	// 1. 返回测试配置的监控结果。
	return m.result, m.err
}

type fakeSubscription struct {
	records []subscription.Record
}

// ListActive 返回测试指定的有效订阅列表。
func (s fakeSubscription) ListActive(context.Context) ([]subscription.Record, error) {
	return s.records, nil
}

type fakeNotification struct{}

// Text 模拟成功发送统一通知。
// 输入：上下文、标题、正文和接收人。
// 输出：返回 nil。
// 副作用：无。
func (fakeNotification) Text(context.Context, []string, string) error {
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
	body   string
	titles [][]string
}

// Text 记录通知标题和正文。
// 输入：上下文、标题层级、正文。
// 输出：始终返回 nil。
// 副作用：覆盖测试替身保存的正文并追加标题。
func (n *captureNotification) Text(_ context.Context, title []string, body string) error {
	// 1. 保存标题和正文供断言。
	n.titles = append(n.titles, append([]string(nil), title...))
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

	// 2. 注册并核对七项固定名称、频率和仅手动属性。
	if err := RegisterAll(registry, dependencies); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 7 {
		t.Fatalf("definition count = %d, want 7", len(definitions))
	}
	wanted := map[string]string{
		"test_crontab": "0 9 * * *", "update_tushare_daily_data": "",
		"sync_investment_articles": "", "check_service_monitors": "0 22 * * *",
		"check_subscription_expiry_notify": "30 9 1 * *", "backup_postgres": "30 3 * * *",
		"rebuild_investment_signal_groups": "",
	}
	for _, definition := range definitions {
		if wanted[definition.Name] != definition.Schedule {
			t.Errorf("schedule %s = %q, want %q", definition.Name, definition.Schedule, wanted[definition.Name])
		}
		if (definition.Name == "rebuild_investment_signal_groups" ||
			definition.Name == "update_tushare_daily_data" ||
			definition.Name == "sync_investment_articles") && !definition.ManualOnly {
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
	if !found || len(registry.Definitions()) != 8 {
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
	if !found || len(registry.Definitions()) != 8 {
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

// TestCheckSubscriptionExpiryNotifySendsMonthlySummary 验证每月有效订阅汇总的正文顺序。
func TestCheckSubscriptionExpiryNotifySendsMonthlySummary(t *testing.T) {
	notification := &captureNotification{}
	taskSet := &tasks{dependencies: Dependencies{
		Subscriptions: fakeSubscription{
			records: []subscription.Record{
				{ServiceName: "较早到期", ExpiresOn: "2026-09-15", DaysUntilExpiry: 14, AnnualFee: "60.00", MonthlyFee: "5.00"},
				{ServiceName: "较晚到期", ExpiresOn: "2028-03-01", DaysUntilExpiry: 546, AnnualFee: "120.00", MonthlyFee: "10.00"},
			},
		},
		Notification:    notification,
		SubscriptionURL: "https://aowugong.example.test/subscriptions",
		Now:             func() time.Time { return time.Date(2026, 9, 1, 9, 30, 0, 0, time.Local) },
	}}

	message, err := taskSet.checkSubscriptionExpiryNotify(context.Background())
	if err != nil {
		t.Fatalf("checkSubscriptionExpiryNotify() error = %v", err)
	}
	if message != "已发送有效订阅月报，共 2 个" {
		t.Errorf("message = %q", message)
	}
	for _, fragment := range []string{"当前共有 2 个有效订阅", "管理：https://aowugong.example.test/subscriptions", "较晚到期", "较早到期"} {
		if !strings.Contains(notification.body, fragment) {
			t.Errorf("notification = %q, missing %q", notification.body, fragment)
		}
	}
	if strings.Index(notification.body, "较早到期") > strings.Index(notification.body, "较晚到期") {
		t.Errorf("notification order = %q", notification.body)
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

// TestCheckServiceMonitorsReportsDownTargets 验证异常服务会带回具体目标和原因，且不额外发送恢复消息。
// 输入：一个从正常变为异常的监控结果。
// 输出：任务返回包含服务名和原因的 error，不发送恢复通知。
// 副作用：写入通知测试替身。
func TestCheckServiceMonitorsReportsDownTargets(t *testing.T) {
	reason := "HTTP 503"
	down := monitoring.Result{
		TargetCode: "vaultwarden", TargetName: "Vaultwarden",
		TargetURL: "https://vault.aowugong.top/", Status: "down", ErrorMessage: &reason,
	}
	notification := &captureNotification{}
	taskSet := &tasks{dependencies: Dependencies{
		Monitoring: fakeMonitor{result: monitoring.CheckResult{
			CheckedCount: 1, DownCount: 1, Results: []monitoring.Result{down},
			Transitions: []monitoring.Transition{{Result: down, PreviousStatus: "up"}},
		}},
		Notification: notification,
		Now:          func() time.Time { return time.Date(2026, 10, 29, 22, 0, 0, 0, time.Local) },
	}}

	message, err := taskSet.checkServiceMonitors(context.Background())
	if err == nil {
		t.Fatal("checkServiceMonitors() error = nil, want error")
	}
	if message != "检查=1，正常=0，异常=1" {
		t.Errorf("message = %q", message)
	}
	for _, fragment := range []string{"发现 1 个服务异常", "Vaultwarden（vaultwarden）", "HTTP 503"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error = %q, missing %q", err.Error(), fragment)
		}
	}
	if len(notification.titles) != 0 {
		t.Fatalf("unexpected notifications: %v", notification.titles)
	}
}

// TestCheckServiceMonitorsSendsRecovery 验证恢复时发送一条合并恢复通知。
// 输入：一个从异常恢复为正常的监控结果。
// 输出：任务成功，并发送“监控 / 服务恢复”标题和恢复正文。
// 副作用：写入通知测试替身。
func TestCheckServiceMonitorsSendsRecovery(t *testing.T) {
	up := monitoring.Result{
		TargetCode: "vaultwarden", TargetName: "Vaultwarden",
		TargetURL: "https://vault.aowugong.top/", Status: "up",
	}
	notification := &captureNotification{}
	taskSet := &tasks{dependencies: Dependencies{
		Monitoring: fakeMonitor{result: monitoring.CheckResult{
			CheckedCount: 1, UpCount: 1, Results: []monitoring.Result{up},
			Transitions: []monitoring.Transition{{Result: up, PreviousStatus: "down"}},
		}},
		Notification: notification,
		Now:          func() time.Time { return time.Date(2026, 10, 29, 22, 10, 0, 0, time.Local) },
	}}

	message, err := taskSet.checkServiceMonitors(context.Background())
	if err != nil {
		t.Fatalf("checkServiceMonitors() error = %v", err)
	}
	if message != "检查=1，正常=1，异常=0" {
		t.Errorf("message = %q", message)
	}
	if len(notification.titles) != 1 {
		t.Fatalf("notifications = %v, want 1", notification.titles)
	}
	if got := strings.Join(notification.titles[0], "/"); got != "监控/服务恢复" {
		t.Errorf("title = %q", got)
	}
	for _, fragment := range []string{"时间：2026-10-29 22:10:00", "Vaultwarden（vaultwarden）已恢复"} {
		if !strings.Contains(notification.body, fragment) {
			t.Errorf("body = %q, missing %q", notification.body, fragment)
		}
	}
}

// TestRebuildInvestmentSignalGroupsAppliesGlobalDictionary 验证手动任务调用全局词典正式应用入口。
// 输入：返回固定摘要的文章服务。
// 输出：任务要求应用六十天重组，并返回组和别名数量。
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
	for _, fragment := range []string{"概念组=2", "别名=3"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message = %q, missing %q", message, fragment)
		}
	}
}
