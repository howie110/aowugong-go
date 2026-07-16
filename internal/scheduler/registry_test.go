package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type fakeNotifier struct {
	body string
}

// Text 记录任务失败通知正文。
// 输入：上下文、标题、正文和接收人。
// 输出：始终返回 nil。
// 副作用：修改测试通知器的 body 字段。
func (n *fakeNotifier) Text(_ context.Context, _ []string, body, _ string) error {
	// 1. 保存正文供断言四段格式。
	n.body = body
	return nil
}

// TestRegistryPreventsConcurrentRunsOfSameJob 验证同名任务不能并发执行。
// 输入：一个等待释放信号的长任务和两次并发 Run。
// 输出：第二次立即返回 ErrAlreadyRunning，第一次释放后成功。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestRegistryPreventsConcurrentRunsOfSameJob(t *testing.T) {
	// 1. 创建注册表并注册可控阻塞任务。
	registry := newTestRegistry(t, &fakeNotifier{})
	started := make(chan struct{})
	release := make(chan struct{})
	if err := registry.Register(Definition{
		Name: "blocking", Schedule: "0 9 * * *", Timeout: time.Second,
		Run: func(ctx context.Context) (string, error) {
			close(started)
			select {
			case <-release:
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	firstResult := make(chan error, 1)
	go func() {
		_, err := registry.Run(context.Background(), "blocking", SourceScheduler)
		firstResult <- err
	}()
	<-started

	// 2. 第二次执行应被拦截，释放第一次后应正常结束。
	_, secondErr := registry.Run(context.Background(), "blocking", SourceManual)
	if !errors.Is(secondErr, ErrAlreadyRunning) {
		t.Fatalf("second Run() error = %v, want ErrAlreadyRunning", secondErr)
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
}

// TestRegistryTimesOutRecordsAndNotifiesFailure 验证超时任务入库并发送四段失败通知。
// 输入：超时时间短于任务等待时间的任务。
// 输出：返回 deadline exceeded，记录 failed 和耗时，并发送标准失败正文。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestRegistryTimesOutRecordsAndNotifiesFailure(t *testing.T) {
	// 1. 创建带可观察通知器的注册表并注册超时任务。
	notifier := &fakeNotifier{}
	registry := newTestRegistry(t, notifier)
	if err := registry.Register(Definition{
		Name: "timeout", Schedule: "0 9 * * *", Timeout: 20 * time.Millisecond,
		Run: func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 2. 执行并核对超时错误和失败通知四段字段。
	_, err := registry.Run(context.Background(), "timeout", SourceCLI)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
	for _, fragment := range []string{"任务：timeout", "时间：", "状态：执行失败", "信息："} {
		if !strings.Contains(notifier.body, fragment) {
			t.Errorf("notification body %q missing %q", notifier.body, fragment)
		}
	}

	// 3. 查询最新执行记录并核对状态、来源和耗时。
	var status, source string
	var duration int64
	if err := registry.db.QueryRowContext(context.Background(), `SELECT status, source, duration_ms
		FROM job_execution WHERE job_id = 'timeout' ORDER BY id DESC LIMIT 1`).Scan(&status, &source, &duration); err != nil {
		t.Fatalf("query job_execution: %v", err)
	}
	if status != "failed" || source != string(SourceCLI) || duration < 1 {
		t.Errorf("execution = status:%q source:%q duration:%d", status, source, duration)
	}
}

// TestRegistryKeepsLockUntilTimedOutJobExits 验证忽略取消的超时任务退出前不会释放同名锁。
// 输入：一个超时后仍等待显式释放信号的任务。
// 输出：超时后第二次运行返回 ErrAlreadyRunning，任务退出后可以再次获取执行权。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestRegistryKeepsLockUntilTimedOutJobExits(t *testing.T) {
	// 1. 注册一个故意忽略上下文、只响应测试释放信号的任务。
	registry := newTestRegistry(t, &fakeNotifier{})
	release := make(chan struct{})
	if err := registry.Register(Definition{
		Name: "stubborn", Schedule: "0 9 * * *", Timeout: 20 * time.Millisecond,
		Run: func(context.Context) (string, error) {
			<-release
			return "released", nil
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 2. 首次运行超时后，同名任务仍应保持正在执行状态。
	if _, err := registry.Run(context.Background(), "stubborn", SourceManual); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Run() error = %v, want deadline exceeded", err)
	}
	if _, err := registry.Run(context.Background(), "stubborn", SourceManual); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Run() error = %v, want ErrAlreadyRunning", err)
	}

	// 3. 业务函数真正退出后轮询确认执行权已经释放。
	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		definition, err := registry.acquire("stubborn")
		if err == nil {
			registry.release(definition.Name)
			break
		}
		if !errors.Is(err, ErrAlreadyRunning) || time.Now().After(deadline) {
			t.Fatalf("acquire() error after release = %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRegistryRecoversPanic 验证任务 panic 被包装器恢复为失败。
// 输入：执行时 panic 的任务。
// 输出：Run 返回带 panic 上下文的错误且进程继续运行。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestRegistryRecoversPanic(t *testing.T) {
	// 1. 创建注册表并注册 panic 任务。
	registry := newTestRegistry(t, &fakeNotifier{})
	if err := registry.Register(Definition{
		Name: "panic", Schedule: "0 9 * * *", Timeout: time.Second,
		Run: func(context.Context) (string, error) {
			panic("boom")
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 2. 执行并核对 panic 已转换成普通错误。
	_, err := registry.Run(context.Background(), "panic", SourceManual)
	if err == nil || !strings.Contains(err.Error(), "panic") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Run() error = %v, want recovered panic", err)
	}
}

// newTestRegistry 创建完成迁移的任务注册表测试实例。
// 输入：t 管理临时目录和失败，notifier 接收失败通知。
// 输出：返回连接独立 MySQL schema 的注册表。
// 副作用：创建并迁移测试 MySQL schema。
func newTestRegistry(t *testing.T, notifier Notifier) *Registry {
	// 1. 打开隔离 MySQL 测试 schema。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 返回丢弃测试日志的注册表。
	return NewRegistry(db, notifier, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
}

type discardWriter struct{}

// Write 丢弃测试日志字节。
// 输入：p 是日志字节。
// 输出：返回完整写入长度和 nil 错误。
// 副作用：无。
func (discardWriter) Write(p []byte) (int, error) {
	// 1. 报告全部字节已处理。
	return len(p), nil
}
