// Package job 注册 finance 与普通业务的定时及手动生产任务。
package job

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	"github.com/howiedata/aowugong-go/internal/githubbackup"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/vaultwardenbackup"
)

const subscriptionReminderDays = 10

// DataUpdater 定义日线更新任务使用的业务入口。
type DataUpdater interface {
	UpdateDaily(ctx context.Context) (financedata.SyncResult, error)
}

// ArticleSyncer 定义文章同步任务使用的业务入口。
type ArticleSyncer interface {
	SyncScheduled(ctx context.Context, classifySignals bool) (articleanalysis.SyncResult, error)
	RebuildSignalGroups(ctx context.Context, days int, apply bool) (articleanalysis.SignalGroupRebuildResult, error)
}

// Monitor 定义服务监控任务使用的业务入口。
type Monitor interface {
	CheckAll(ctx context.Context) (monitoring.CheckResult, error)
}

// SubscriptionLister 定义订阅提醒任务使用的到期筛选入口。
type SubscriptionLister interface {
	ListExpiring(ctx context.Context, reminderDays int) ([]subscription.Record, error)
}

// NotificationSender 定义业务任务需要的统一文本通知入口。
type NotificationSender interface {
	Text(ctx context.Context, titleParts []string, body, to string) error
}

// GitHubCodeBackuper 定义固定白名单仓库的代码冷备份入口。
type GitHubCodeBackuper interface {
	Backup(ctx context.Context) (githubbackup.Result, error)
}

// VaultwardenBackupMailer 定义加密并邮件发送最新密码库备份的入口。
type VaultwardenBackupMailer interface {
	SendLatest(ctx context.Context) (vaultwardenbackup.Result, error)
}

// BackupFunc 定义可测试的 PostgreSQL 备份函数。
type BackupFunc func(ctx context.Context, directory string, retention int, now time.Time) (string, error)

// Dependencies 汇总全部任务所需的显式业务依赖与备份配置。
type Dependencies struct {
	DB                *sql.DB
	Data              DataUpdater
	Articles          ArticleSyncer
	Monitoring        Monitor
	Subscriptions     SubscriptionLister
	Notification      NotificationSender
	GitHubBackup      GitHubCodeBackuper
	VaultwardenBackup VaultwardenBackupMailer
	BackupDir         string
	BackupRetention   int
	Now               func() time.Time
	Backup            BackupFunc
}

type tasks struct {
	dependencies Dependencies
}

// RegisterAll 把固定定时任务和手动任务注册到唯一任务注册表。
// 输入：registry 是统一执行入口，dependencies 是业务服务和备份配置。
// 输出：全部注册成功返回 nil，依赖或定义无效时返回错误。
// 副作用：修改进程内任务注册表，不立即执行任务。
func RegisterAll(registry *scheduler.Registry, dependencies Dependencies) error {
	// 1. 校验任务依赖。
	if registry == nil || dependencies.DB == nil || dependencies.Data == nil || dependencies.Articles == nil ||
		dependencies.Monitoring == nil || dependencies.Subscriptions == nil || dependencies.Notification == nil {
		return fmt.Errorf("任务注册依赖不完整")
	}
	if dependencies.BackupDir == "" || dependencies.BackupRetention < 1 || dependencies.Backup == nil {
		return fmt.Errorf("PostgreSQL 备份配置无效")
	}
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	taskSet := &tasks{dependencies: dependencies}

	// 2. 逐项注册固定名称、上海时区 Cron 表达式和业务超时。
	definitions := []scheduler.Definition{
		{Name: "test_crontab", Description: "每日任务链路测试", Schedule: "0 9 * * *", Timeout: time.Minute, Run: taskSet.testCrontab},
		{Name: "update_tushare_daily_data", Description: "更新 Tushare 日线数据", ManualOnly: true, Timeout: 2 * time.Hour, Run: taskSet.updateTushareDailyData},
		{Name: "sync_investment_articles", Description: "手动同步并分析投资文章", ManualOnly: true, ConcurrencyKey: "investment_signal_groups", Timeout: 3 * time.Hour, Run: taskSet.syncInvestmentArticles},
		{Name: "rebuild_investment_signal_groups", Description: "全局重建投资信号概念组", ManualOnly: true, ConcurrencyKey: "investment_signal_groups", Timeout: 30 * time.Minute, Run: taskSet.rebuildInvestmentSignalGroups},
		{Name: "check_service_monitors", Description: "检查服务连通性", Schedule: "0 22 * * *", Timeout: 10 * time.Minute, Run: taskSet.checkServiceMonitors},
		{Name: "check_subscription_expiry_notify", Description: "检查订阅到期并提醒", Schedule: "30 9 * * *", Timeout: 10 * time.Minute, Run: taskSet.checkSubscriptionExpiryNotify},
		{Name: "openilink_reply_reminder", Description: "提醒回复 OpeniLink Bot", Schedule: "0 10 * * *", Timeout: 5 * time.Minute, Run: taskSet.openILinkReplyReminder},
		{Name: "backup_postgres", Description: "创建 PostgreSQL 一致性备份", Schedule: "30 3 * * *", Timeout: 2 * time.Hour, Run: taskSet.backupPostgres},
	}
	if dependencies.GitHubBackup != nil {
		definitions = append(definitions, scheduler.Definition{
			Name: "backup_github_code", Description: "备份账号自有及固定组织 GitHub 仓库",
			Schedule: "0 4 * * 0", Timeout: 3 * time.Hour, Run: taskSet.backupGitHubCode,
		})
	}
	if dependencies.VaultwardenBackup != nil {
		definitions = append(definitions, scheduler.Definition{
			Name: "email_vaultwarden_backup", Description: "加密并邮件发送最新 Vaultwarden 备份",
			Schedule: "0 5 * * 0", Timeout: 30 * time.Minute, Run: taskSet.emailVaultwardenBackup,
		})
	}
	for _, definition := range definitions {
		if err := registry.Register(definition); err != nil {
			return fmt.Errorf("注册任务 %s: %w", definition.Name, err)
		}
	}
	return nil
}

// testCrontab 执行每日任务链路自检。
// 输入：ctx 是任务超时上下文。
// 输出：返回当前时间的成功摘要。
// 副作用：读取系统时钟，不访问数据库或外部接口。
func (t *tasks) testCrontab(ctx context.Context) (string, error) {
	// 1. 在返回前响应已取消上下文。
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("任务链路测试取消: %w", err)
	}
	return "任务链路正常，时间=" + t.dependencies.Now().Format("2006-01-02 15:04:05"), nil
}

// updateTushareDailyData 补齐缺失开市日的股票日线。
// 输入：ctx 是任务超时上下文。
// 输出：返回缺失日期、同步日期和行数摘要；失败时返回错误。
// 副作用：调用 Tushare HTTP 并写入 PostgreSQL tushare_daily。
func (t *tasks) updateTushareDailyData(ctx context.Context) (string, error) {
	// 1. 调用唯一日线同步服务并格式化任务摘要。
	result, err := t.dependencies.Data.UpdateDaily(ctx)
	if err != nil {
		return "", fmt.Errorf("更新 Tushare 日线: %w", err)
	}
	return fmt.Sprintf("窗口=%s~%s，缺失=%d，同步=%d，空响应=%d，写入=%d",
		result.StartDate, result.EndDate, result.MissingCount, result.SyncedCount, result.EmptyCount, result.RowCount), nil
}

// syncInvestmentArticles 同步微信读书公众号并分析当前批次待处理文章。
// 输入：ctx 是任务超时上下文。
// 输出：返回抓取、写入和分析摘要；来源或模型失败时返回错误。
// 副作用：调用微信读书、微信公众号原文、DeepSeek 并写入 PostgreSQL。
func (t *tasks) syncInvestmentArticles(ctx context.Context) (string, error) {
	// 1. 页面手动执行优先快速返回，自动和 CLI 执行同时完成信号归类。
	classifySignals := scheduler.SourceFromContext(ctx) != scheduler.SourceManual
	result, err := t.dependencies.Articles.SyncScheduled(ctx, classifySignals)
	message := fmt.Sprintf("来源=%d，抓取=%d，新增=%d，更新=%d，分析=%d，归类=%d，跳过=%d，错误=%d",
		result.SourceCount, result.FetchedCount, result.InsertedCount, result.UpdatedCount,
		result.AnalyzedCount, result.ClassifiedAliasCount, result.SkippedCount, result.ErrorCount)

	// 2. 保留已完成摘要并把业务错误交给统一包装器通知。
	if err != nil {
		return message, fmt.Errorf("同步投资文章: %w", err)
	}
	return message, nil
}

// rebuildInvestmentSignalGroups 全局收敛六十天投资信号并替换概念词典。
// 输入：ctx 是任务超时上下文。
// 输出：返回新概念组、别名和待归类数量；模型或事务失败时返回错误。
// 副作用：调用 DeepSeek，并在单个事务内重建 PostgreSQL 信号概念词典。
func (t *tasks) rebuildInvestmentSignalGroups(ctx context.Context) (string, error) {
	// 1. 调用文章服务唯一全局重组入口并明确应用已校验结果。
	result, err := t.dependencies.Articles.RebuildSignalGroups(ctx, articleanalysis.DefaultTargetDays, true)
	message := fmt.Sprintf("概念组=%d，别名=%d，待归类=%d", result.GroupCount, result.AliasCount, result.PendingAliasCount)
	if err != nil {
		return message, fmt.Errorf("重建投资信号概念组: %w", err)
	}
	if !result.Applied {
		return message, fmt.Errorf("投资信号概念组重建未应用")
	}
	return message, nil
}

// checkServiceMonitors 探测当前全部服务并持久化结果。
// 输入：ctx 是任务超时上下文。
// 输出：全部正常时返回计数摘要，存在 down 时返回错误。
// 副作用：调用监控目标并写入 PostgreSQL service_monitor_result。
func (t *tasks) checkServiceMonitors(ctx context.Context) (string, error) {
	// 1. 调用统一监控服务并按 down 数决定任务状态。
	result, err := t.dependencies.Monitoring.CheckAll(ctx)
	if err != nil {
		return "", fmt.Errorf("检查服务监控: %w", err)
	}
	message := fmt.Sprintf("检查=%d，正常=%d，异常=%d", result.CheckedCount, result.UpCount, result.DownCount)
	if result.DownCount > 0 {
		return message, fmt.Errorf("发现 %d 个服务异常", result.DownCount)
	}
	return message, nil
}

// checkSubscriptionExpiryNotify 检查正好十天后到期的订阅并发送微信提醒。
// 输入：ctx 是任务超时上下文。
// 输出：返回检查或提醒数量；查询和发送失败时返回错误。
// 副作用：读取 PostgreSQL，命中记录时调用统一通知服务发送微信并写日志。
func (t *tasks) checkSubscriptionExpiryNotify(ctx context.Context) (string, error) {
	// 1. 复用订阅服务的业务日期和精确到期筛选。
	records, err := t.dependencies.Subscriptions.ListExpiring(ctx, subscriptionReminderDays)
	if err != nil {
		return "", fmt.Errorf("检查订阅到期: %w", err)
	}
	if len(records) == 0 {
		return "没有距离到期 10 天的订阅", nil
	}

	// 2. 使用统一通知服务发送一条合并提醒。
	body := buildSubscriptionBody(records, t.dependencies.Now())
	if err := t.dependencies.Notification.Text(ctx, []string{"AOWUGONG", "SUBSCRIPTION", "EXPIRY"}, body, ""); err != nil {
		return "", fmt.Errorf("发送订阅到期提醒: %w", err)
	}
	return fmt.Sprintf("已提醒 %d 个十天后到期的订阅", len(records)), nil
}

// openILinkReplyReminder 发送每日 OpeniLink 回复保活提醒。
// 输入：ctx 是任务超时上下文。
// 输出：发送成功返回摘要，失败时返回错误。
// 副作用：调用统一通知服务发送微信并写 PostgreSQL 日志。
func (t *tasks) openILinkReplyReminder(ctx context.Context) (string, error) {
	// 1. 发送固定短提醒以维持微信消息窗口。
	body := "OpeniLink 每日保活提醒：请回复机器人任意一句，保持微信通知通道可用。"
	if err := t.dependencies.Notification.Text(ctx, []string{"AOWUGONG", "OPENILINK", "REMINDER"}, body, ""); err != nil {
		return "", fmt.Errorf("发送 OpeniLink 回复提醒: %w", err)
	}
	return "OpeniLink 回复提醒已发送", nil
}

// backupPostgres 创建应用数据库一致性备份并执行保留策略。
// 输入：ctx 是任务超时上下文。
// 输出：返回新快照路径；创建、验证或清理失败时返回错误。
// 副作用：调用 pg_dump 读取 PostgreSQL 一致视图，创建文件并删除超额旧备份。
func (t *tasks) backupPostgres(ctx context.Context) (string, error) {
	// 1. 调用数据库包唯一备份入口并返回产物路径。
	path, err := t.dependencies.Backup(ctx, t.dependencies.BackupDir,
		t.dependencies.BackupRetention, t.dependencies.Now())
	if err != nil {
		return "", fmt.Errorf("备份 PostgreSQL: %w", err)
	}
	return "PostgreSQL 备份=" + path, nil
}

// backupGitHubCode 更新固定白名单裸仓库并发送一次成功摘要。
// 输入：ctx 是任务超时上下文。
// 输出：返回发现、新增、更新、失联和失败数量；Git 或通知失败时返回错误。
// 副作用：调用 GitHub、写入代码备份目录，成功时通过统一通知服务发送微信。
func (t *tasks) backupGitHubCode(ctx context.Context) (string, error) {
	// 1. 调用唯一代码备份服务并保留部分完成的计数摘要。
	result, err := t.dependencies.GitHubBackup.Backup(ctx)
	message := fmt.Sprintf("仓库=%d，新增=%d，更新=%d，失联=%d，失败=%d",
		result.DiscoveredCount, result.CreatedCount, result.UpdatedCount,
		result.UnavailableCount, result.FailedCount)
	if err != nil {
		return message, fmt.Errorf("备份 GitHub 代码: %w", err)
	}

	// 2. 成功时发送一条四段式微信摘要，失败通知由任务统一包装器负责。
	body := strings.Join([]string{
		"- 任务：backup_github_code",
		"- 时间：" + t.dependencies.Now().Format("2006-01-02 15:04:05"),
		"- 状态：执行成功",
		"- 信息：" + message,
	}, "\n")
	if err := t.dependencies.Notification.Text(ctx,
		[]string{"AOWUGONG", "GITHUB", "BACKUP"}, body, ""); err != nil {
		return message, fmt.Errorf("发送 GitHub 代码备份成功通知: %w", err)
	}
	return message, nil
}

// emailVaultwardenBackup 加密最新 Vaultwarden 备份并发送到异地邮箱。
// 输入：ctx 是任务超时上下文。
// 输出：返回源文件、大小和收件人摘要；加密或邮件发送失败时返回错误。
// 副作用：读取服务器备份、创建临时加密文件并调用 SMTP 发送附件。
func (t *tasks) emailVaultwardenBackup(ctx context.Context) (string, error) {
	// 1. 调用唯一备份邮件服务并把结果整理为任务执行摘要。
	result, err := t.dependencies.VaultwardenBackup.SendLatest(ctx)
	message := fmt.Sprintf("文件=%s，大小=%d，收件人=%s",
		result.SourcePath, result.Size, result.EmailTo)
	if err != nil {
		return message, fmt.Errorf("发送 Vaultwarden 加密备份: %w", err)
	}
	return message, nil
}

// buildSubscriptionBody 组装订阅到期微信提醒正文。
// 输入：records 是十天后到期记录，now 是通知时间。
// 输出：返回按记录逐行展示的正文。
// 副作用：无。
func buildSubscriptionBody(records []subscription.Record, now time.Time) string {
	// 1. 写入时间、状态并逐条追加费用与剩余天数。
	lines := []string{
		"- 时间：" + now.Format("2006-01-02 15:04:05"),
		fmt.Sprintf("- 状态：发现 %d 个订阅将在 %d 天后到期", len(records), subscriptionReminderDays),
		"",
	}
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("- %s：%s 到期，剩余 %d 天，年费 %s，月费 %s",
			record.ServiceName, record.ExpiresOn, record.DaysUntilExpiry, record.AnnualFee, record.MonthlyFee))
	}
	return strings.Join(lines, "\n")
}
