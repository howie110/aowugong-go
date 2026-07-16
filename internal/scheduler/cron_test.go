package scheduler

import (
	"context"
	"testing"
	"time"
)

// TestCronSchedulerRegistersDefinitionsAndStops 验证注册表定义能装载到上海时区 Cron 并优雅停止。
// 输入：一个已注册的五段 Cron 任务。
// 输出：Start 装载一个条目，Stop 在上下文内完成。
// 副作用：启动并停止测试进程内 Cron 调度器。
func TestCronSchedulerRegistersDefinitionsAndStops(t *testing.T) {
	// 1. 复用任务包装器测试注册表并添加一个不会在测试期间触发的任务。
	registry := newTestRegistry(t, &fakeNotifier{})
	if err := registry.Register(Definition{
		Name: "daily", Schedule: "0 9 * * *", Timeout: time.Minute,
		Run: func(context.Context) (string, error) { return "ok", nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	scheduler := NewCronScheduler(registry, location)

	// 2. 启动并核对条目数量，再优雅停止。
	if err := scheduler.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(scheduler.cron.Entries()) != 1 {
		t.Errorf("entry count = %d, want 1", len(scheduler.cron.Entries()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestCronSchedulerStopCancelsRunningJob 验证停止调度器会取消已经派发的任务上下文。
// 输入：一个每秒触发并等待上下文取消的任务。
// 输出：Stop 取消任务并在关闭时限内返回成功。
// 副作用：启动短生命周期 Cron 并写入测试 MySQL 执行记录。
func TestCronSchedulerStopCancelsRunningJob(t *testing.T) {
	// 1. 注册每秒触发的协作任务并等待它开始执行。
	registry := newTestRegistry(t, &fakeNotifier{})
	started := make(chan struct{})
	finished := make(chan struct{})
	if err := registry.Register(Definition{
		Name: "cancel-on-stop", Schedule: "@every 1s", Timeout: time.Minute,
		Run: func(ctx context.Context) (string, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return "", ctx.Err()
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	scheduler := NewCronScheduler(registry, time.UTC)
	if err := scheduler.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Cron task did not start")
	}

	// 2. 停止调度器应取消根上下文并等待任务完成。
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("running task was not canceled")
	}
}
