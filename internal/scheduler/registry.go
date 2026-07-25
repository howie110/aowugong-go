// Package scheduler 提供自动、手动和 CLI 共用的任务注册与执行包装器。
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrAlreadyRunning 表示同名任务已有实例尚未退出。
var ErrAlreadyRunning = errors.New("同名任务正在执行")

// Source 表示任务由调度器、页面手动操作或 CLI 发起。
type Source string

type sourceContextKey struct{}

const (
	// SourceScheduler 表示 Go 内嵌定时调度触发。
	SourceScheduler Source = "scheduler"
	// SourceManual 表示页面或 HTTP 接口手动触发。
	SourceManual Source = "manual"
	// SourceCLI 表示命令行显式触发。
	SourceCLI Source = "cli"
)

// WithSource 把统一任务执行来源写入业务上下文。
// 输入：ctx 是基础上下文，source 是 scheduler、manual 或 cli。
// 输出：返回携带来源的新上下文。
// 副作用：无，不修改原上下文。
func WithSource(ctx context.Context, source Source) context.Context {
	// 1. 使用包私有键保存类型化来源，避免与业务上下文字段冲突。
	return context.WithValue(ctx, sourceContextKey{}, source)
}

// SourceFromContext 读取统一任务执行来源。
// 输入：ctx 是任务包装器传给业务函数的上下文。
// 输出：返回 scheduler、manual、cli 或空值。
// 副作用：无。
func SourceFromContext(ctx context.Context) Source {
	// 1. 只接受当前包写入的 Source 类型值。
	source, _ := ctx.Value(sourceContextKey{}).(Source)
	return source
}

// JobFunc 定义统一任务函数，返回摘要消息或错误。
type JobFunc func(ctx context.Context) (string, error)

// Notifier 定义任务包装器需要的统一文本通知能力。
type Notifier interface {
	Text(ctx context.Context, titleParts []string, body, to string) error
}

// Definition 描述注册任务的名称、Cron、手动属性、业务互斥键、超时和入口。
type Definition struct {
	Name           string
	Description    string
	Schedule       string
	ManualOnly     bool
	ConcurrencyKey string
	Timeout        time.Duration
	Run            JobFunc
}

// Result 描述一次任务执行的最终状态和耗时。
type Result struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	Source     Source        `json:"source"`
	Status     string        `json:"status"`
	Message    string        `json:"message,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
}

// Registry 保存唯一任务定义并提供统一执行包装器。
type Registry struct {
	db           *sql.DB
	notifier     Notifier
	logger       *slog.Logger
	databaseLock bool
	mutex        sync.Mutex
	definitions  map[string]Definition
	running      map[string]bool
}

// RegistryOption 调整任务注册表的基础设施行为。
type RegistryOption func(*Registry)

type jobOutcome struct {
	message string
	err     error
}

// NewRegistry 创建任务注册表。
// 输入：db 写入执行结果，notifier 发送失败微信，logger 记录开始结束日志。
// 输出：返回可并发复用的空注册表。
// 副作用：无，不执行任务或 SQL。
func NewRegistry(db *sql.DB, notifier Notifier, logger *slog.Logger, options ...RegistryOption) *Registry {
	// 1. 应用标准日志默认值并初始化受锁保护的映射。
	if logger == nil {
		logger = slog.Default()
	}
	registry := &Registry{
		db: db, notifier: notifier, logger: logger,
		databaseLock: true, definitions: make(map[string]Definition), running: make(map[string]bool),
	}

	// 2. 应用测试或特殊基础设施显式提供的选项。
	for _, option := range options {
		if option != nil {
			option(registry)
		}
	}
	return registry
}

// WithoutDatabaseLock 关闭跨进程 SQLite 任务锁。
// 输入：无。
// 输出：返回只供 SQL mock 测试使用的注册表选项。
// 副作用：应用后任务仅保留当前进程内互斥，不得用于正式运行时。
func WithoutDatabaseLock() RegistryOption {
	// 1. 返回显式关闭数据库锁的配置函数。
	return func(registry *Registry) {
		registry.databaseLock = false
	}
}

// Register 注册一个可由所有来源复用的任务定义。
// 输入：definition 包含唯一名称、可选 Cron、手动属性、可选共享互斥键、超时和业务函数。
// 输出：注册成功返回 nil，定义无效或重名时返回错误。
// 副作用：修改进程内任务注册表。
func (r *Registry) Register(definition Definition) error {
	// 1. 规范化并校验任务定义的必需字段。
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Schedule = strings.TrimSpace(definition.Schedule)
	definition.ConcurrencyKey = strings.TrimSpace(definition.ConcurrencyKey)
	if definition.Name == "" || definition.Run == nil {
		return fmt.Errorf("任务名称和执行函数不能为空")
	}
	if definition.ManualOnly && definition.Schedule != "" {
		return fmt.Errorf("仅手动任务 %s 不能配置 Cron 表达式", definition.Name)
	}
	if !definition.ManualOnly && definition.Schedule == "" {
		return fmt.Errorf("定时任务 %s 的 Cron 表达式不能为空", definition.Name)
	}
	if definition.ConcurrencyKey == "" {
		definition.ConcurrencyKey = definition.Name
	}
	if definition.Timeout <= 0 {
		return fmt.Errorf("任务 %s 超时必须大于零", definition.Name)
	}

	// 2. 在锁内拒绝重名并保存唯一定义。
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, exists := r.definitions[definition.Name]; exists {
		return fmt.Errorf("任务 %s 已注册", definition.Name)
	}
	r.definitions[definition.Name] = definition
	return nil
}

// Definitions 返回按名称排序的任务定义副本。
// 输入：无。
// 输出：返回当前全部定义，调用方修改不会影响注册表。
// 副作用：无。
func (r *Registry) Definitions() []Definition {
	// 1. 在锁内复制定义，避免暴露内部映射。
	r.mutex.Lock()
	result := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		result = append(result, definition)
	}
	r.mutex.Unlock()

	// 2. 使用稳定名称顺序便于页面、CLI 和测试展示。
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// Run 通过统一包装器执行一个已注册任务。
// 输入：ctx 控制调用，name 是任务名，source 是 scheduler/manual/cli。
// 输出：返回执行状态；并发、超时、panic 或业务失败时返回错误。
// 副作用：执行任务、写 SQLite、记录日志，失败时发送微信通知。
func (r *Registry) Run(ctx context.Context, name string, source Source) (Result, error) {
	// 1. 查找定义并原子获取同名任务执行权。
	definition, err := r.acquire(name)
	if err != nil {
		return Result{}, err
	}
	if source != SourceScheduler && source != SourceManual && source != SourceCLI {
		r.release(definition.ConcurrencyKey)
		return Result{}, fmt.Errorf("任务 %s 来源无效: %q", definition.Name, source)
	}
	if definition.ManualOnly && source == SourceScheduler {
		r.release(definition.ConcurrencyKey)
		return Result{}, fmt.Errorf("任务 %s 仅允许手动或 CLI 执行", definition.Name)
	}
	unlockDatabase := func() error { return nil }
	if r.databaseLock {
		unlockDatabase, err = acquireSQLiteLock(ctx, r.db, definition.ConcurrencyKey, definition.Timeout)
		if err != nil {
			r.release(definition.ConcurrencyKey)
			return Result{}, err
		}
	}
	release := func() {
		if err := unlockDatabase(); err != nil {
			r.logger.Error("释放任务数据库锁失败", "job", definition.Name, "error", err)
		}
		r.release(definition.ConcurrencyKey)
	}
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			release()
		}
	}()
	// 2. 写入 running 记录并建立任务超时上下文。
	startedAt := time.Now()
	executionID, err := r.startExecution(ctx, definition.Name, source, startedAt)
	if err != nil {
		return Result{}, err
	}
	r.logger.Info("任务开始", "job", definition.Name, "source", source, "execution_id", executionID)
	runContext, cancel := context.WithTimeout(WithSource(ctx, source), definition.Timeout)
	defer cancel()
	outcomes := make(chan jobOutcome, 1)
	releaseOnReturn = false
	go func() {
		message, runErr := execute(runContext, definition.Run)
		release()
		outcomes <- jobOutcome{message: message, err: runErr}
	}()

	// 3. 等待业务完成或超时；超时后保持并发锁直到业务函数真正退出。
	var outcome jobOutcome
	select {
	case outcome = <-outcomes:
	case <-runContext.Done():
		outcome.err = runContext.Err()
	}

	// 4. 更新最终执行结果并记录结束日志。
	finishedAt := time.Now()
	result := Result{
		ID: executionID, Name: definition.Name, Source: source, Status: "success",
		Message: outcome.message, StartedAt: startedAt, FinishedAt: finishedAt,
		Duration: finishedAt.Sub(startedAt),
	}
	if outcome.err != nil {
		result.Status = "failed"
	}
	if err := r.finishExecution(context.Background(), result, outcome.err); err != nil {
		return result, err
	}
	r.logger.Info("任务结束", "job", definition.Name, "status", result.Status, "duration", result.Duration)

	// 5. 失败时发送固定四段微信通知，并返回原始任务错误。
	if outcome.err != nil {
		r.notifyFailure(definition.Name, finishedAt, outcome.err)
		return result, fmt.Errorf("执行任务 %s: %w", definition.Name, outcome.err)
	}
	return result, nil
}

// acquire 查找任务定义并标记为正在执行。
// 输入：name 是任务名。
// 输出：返回定义；不存在或正在执行时返回错误。
// 副作用：修改进程内同名任务执行状态。
func (r *Registry) acquire(name string) (Definition, error) {
	// 1. 在同一把锁中完成查找和并发状态切换。
	name = strings.TrimSpace(name)
	r.mutex.Lock()
	defer r.mutex.Unlock()
	definition, exists := r.definitions[name]
	if !exists {
		return Definition{}, fmt.Errorf("未注册任务 %s", name)
	}
	if r.running[definition.ConcurrencyKey] {
		return Definition{}, fmt.Errorf("任务 %s: %w", name, ErrAlreadyRunning)
	}
	r.running[definition.ConcurrencyKey] = true
	return definition, nil
}

// release 清除业务互斥键的正在执行标记。
// 输入：key 是任务名或多个任务共享的互斥键。
// 输出：无。
// 副作用：修改进程内执行状态并允许同键任务下次运行。
func (r *Registry) release(key string) {
	// 1. 在锁内删除状态，避免并发读写映射。
	r.mutex.Lock()
	delete(r.running, key)
	r.mutex.Unlock()
}

// execute 调用任务并把 panic 恢复成带堆栈的错误。
// 输入：ctx 是超时上下文，job 是业务函数。
// 输出：返回业务消息和错误，panic 时返回恢复错误。
// 副作用：执行传入任务函数。
func execute(ctx context.Context, job JobFunc) (message string, err error) {
	// 1. 延迟恢复任何 panic，避免任务终止服务进程。
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("任务 panic: %v\n%s", value, debug.Stack())
		}
	}()

	// 2. 调用唯一业务入口并原样返回结果。
	return job(ctx)
}

// startExecution 写入任务开始记录并返回主键。
// 输入：ctx 控制写入，name、source 和 startedAt 描述执行。
// 输出：返回自增主键；写入失败时返回错误。
// 副作用：向 SQLite job_execution 新增 running 记录。
func (r *Registry) startExecution(ctx context.Context, name string, source Source, startedAt time.Time) (int64, error) {
	// 1. 写入统一开始状态并读取主键。
	result, err := r.db.ExecContext(ctx, `INSERT INTO job_execution(
		job_id, status, started_at, source, created_at
	) VALUES(?,?,?,?,?)`, name, "running", startedAt, string(source), startedAt)
	if err != nil {
		return 0, fmt.Errorf("记录任务 %s 开始: %w", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("读取任务 %s 执行编号: %w", name, err)
	}
	return id, nil
}

// finishExecution 更新任务最终状态、耗时、消息和错误。
// 输入：ctx 控制写入，result 是执行结果，runErr 是业务错误。
// 输出：成功返回 nil，更新失败返回错误。
// 副作用：更新 SQLite job_execution 指定记录。
func (r *Registry) finishExecution(ctx context.Context, result Result, runErr error) error {
	// 1. 将可选错误转换为 SQL NULL 并更新唯一执行记录。
	var errorMessage any
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE job_execution SET
		status = ?, finished_at = ?, duration_ms = ?, message = ?, error_message = ? WHERE id = ?`,
		result.Status, result.FinishedAt, result.Duration.Milliseconds(),
		result.Message, errorMessage, result.ID)
	if err != nil {
		return fmt.Errorf("记录任务 %s 结果: %w", result.Name, err)
	}
	return nil
}

// notifyFailure 使用固定四段格式发送任务失败微信。
// 输入：name 是任务名，failedAt 是失败时间，runErr 是任务错误。
// 输出：无，通知自身失败只写日志，不覆盖任务错误。
// 副作用：调用统一 notification service 发送微信并写通知日志。
func (r *Registry) notifyFailure(name string, failedAt time.Time, runErr error) {
	// 1. 未配置通知服务时记录警告并结束。
	if r.notifier == nil {
		r.logger.Warn("任务失败通知未配置", "job", name)
		return
	}

	// 2. 使用独立超时上下文确保任务超时后仍可发送告警。
	body := fmt.Sprintf("- 任务：%s\n- 时间：%s\n- 状态：执行失败\n- 信息：\n%s",
		name, failedAt.Format("2006-01-02 15:04:05"), runErr.Error())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.notifier.Text(ctx, []string{"AOWUGONG", "JOB", "ERROR", name}, body, ""); err != nil {
		r.logger.Error("发送任务失败通知失败", "job", name, "error", err)
	}
}
