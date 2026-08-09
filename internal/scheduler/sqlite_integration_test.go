package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestRegistryPreventsCrossRegistryRunsWithDatabaseLock 验证本地 CLI 与服务器调度器不能并发执行同名任务。
// 输入：连接同一隔离 SQLite schema 的两个独立任务注册表。
// 输出：首个任务持锁时第二个注册表返回 ErrAlreadyRunning，释放后首个成功。
// 副作用：创建隔离 SQLite、写入任务执行记录并使用跨连接任务锁。
func TestRegistryPreventsCrossRegistryRunsWithDatabaseLock(t *testing.T) {
	// 1. 创建两个没有共享进程内状态的注册表。
	db := testdatabase.Open(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	first := NewRegistry(db, nil, logger)
	second := NewRegistry(db, nil, logger)
	started := make(chan struct{})
	release := make(chan struct{})
	if err := first.Register(Definition{
		Name: "cross_process", Schedule: "0 9 * * *", Timeout: time.Second,
		Run: func(context.Context) (string, error) {
			close(started)
			<-release
			return "done", nil
		},
	}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if err := second.Register(Definition{
		Name: "cross_process", Schedule: "0 9 * * *", Timeout: time.Second,
		Run: func(context.Context) (string, error) { return "unexpected", nil },
	}); err != nil {
		t.Fatalf("second Register() error = %v", err)
	}

	// 2. 让首个注册表持有数据库锁，再从独立注册表尝试执行。
	firstDone := make(chan error, 1)
	go func() {
		_, err := first.Run(context.Background(), "cross_process", SourceScheduler)
		firstDone <- err
	}()
	<-started
	_, err := second.Run(context.Background(), "cross_process", SourceCLI)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Run() error = %v, want ErrAlreadyRunning", err)
	}

	// 3. 释放首个任务并确认完整执行成功。
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
}
