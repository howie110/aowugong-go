package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestAcquireDatabaseLockRejectsBusyJob 验证未过期任务锁不会被另一执行者抢占。
// 输入：一把已经持有的 SQLite 任务锁。
// 输出：第二次获取返回 ErrAlreadyRunning，释放后可以重新获取。
// 副作用：写入并删除隔离 SQLite 的 job_execution_lock。
func TestAcquireDatabaseLockRejectsBusyJob(t *testing.T) {
	// 1. 获取测试任务锁并保持不释放。
	db := testdatabase.Open(t)
	firstUnlock, err := acquireDatabaseLock(context.Background(), db, "busy", time.Minute)
	if err != nil {
		t.Fatalf("first acquireDatabaseLock() error = %v", err)
	}

	// 2. 第二个执行者必须被统一并发错误拒绝。
	_, err = acquireDatabaseLock(context.Background(), db, "busy", time.Minute)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquireDatabaseLock() error = %v, want ErrAlreadyRunning", err)
	}

	// 3. 释放当前所有者后同名任务可以再次获取。
	if err := firstUnlock(); err != nil {
		t.Fatalf("first unlock error = %v", err)
	}
	secondUnlock, err := acquireDatabaseLock(context.Background(), db, "busy", time.Minute)
	if err != nil {
		t.Fatalf("third acquireDatabaseLock() error = %v", err)
	}
	if err := secondUnlock(); err != nil {
		t.Fatalf("second unlock error = %v", err)
	}
}

// TestAcquireDatabaseLockReclaimsExpiredJob 验证超时遗留锁可被新执行者接管。
// 输入：一条已经过期的数据库锁记录。
// 输出：获取成功并由新所有者释放记录。
// 副作用：写入并删除隔离 SQLite 的 job_execution_lock。
func TestAcquireDatabaseLockReclaimsExpiredJob(t *testing.T) {
	// 1. 写入一个确定过期的遗留锁。
	db := testdatabase.Open(t)
	if _, err := db.Exec(`INSERT INTO job_execution_lock(lock_name, owner_token, acquired_at, expires_at)
		VALUES('aowugong:job:expired', 'old-owner', '2000-01-01 00:00:00', '2000-01-01 00:00:00')`); err != nil {
		t.Fatalf("insert expired lock: %v", err)
	}

	// 2. 新执行者应原子替换过期所有者。
	unlock, err := acquireDatabaseLock(context.Background(), db, "expired", time.Minute)
	if err != nil {
		t.Fatalf("acquireDatabaseLock() error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock error = %v", err)
	}
}
