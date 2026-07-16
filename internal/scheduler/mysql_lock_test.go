package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestAcquireMySQLLockHoldsDedicatedConnection 验证任务锁使用 MySQL 会话锁并显式释放。
// 输入：返回成功锁标记的模拟 MySQL 连接。
// 输出：锁名带应用前缀，释放函数调用 RELEASE_LOCK。
// 副作用：只操作模拟数据库期望，不连接真实 MySQL。
func TestAcquireMySQLLockHoldsDedicatedConnection(t *testing.T) {
	// 1. 准备获取和释放同一会话锁的 SQL 期望。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, 0\)`).
		WithArgs("aowugong:job:heavy").
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(1))
	mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).
		WithArgs("aowugong:job:heavy").
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))

	// 2. 获取锁后调用释放函数，连接必须保持为同一会话。
	unlock, err := acquireMySQLLock(context.Background(), db, "heavy")
	if err != nil {
		t.Fatalf("acquireMySQLLock() error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock() error = %v", err)
	}

	// 3. 断言所有数据库交互都按顺序完成。
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

// TestAcquireMySQLLockRejectsBusyJob 验证其他进程持锁时返回统一并发错误。
// 输入：GET_LOCK 返回 0 的模拟 MySQL 连接。
// 输出：错误可由 errors.Is 识别为 ErrAlreadyRunning。
// 副作用：只操作模拟数据库期望，不连接真实 MySQL。
func TestAcquireMySQLLockRejectsBusyJob(t *testing.T) {
	// 1. 准备已被其他会话占用的锁响应。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, 0\)`).
		WithArgs("aowugong:job:busy").
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(0))

	// 2. 获取锁并断言转换成统一任务并发错误。
	_, err = acquireMySQLLock(context.Background(), db, "busy")
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("acquireMySQLLock() error = %v, want ErrAlreadyRunning", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}
