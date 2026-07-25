package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const sqliteJobLockPrefix = "aowugong:job:"

// acquireSQLiteLock 尝试获取带自动过期时间的跨进程任务锁。
// 输入：ctx 控制写入，db 是应用 SQLite，jobName 是业务互斥键，ttl 是最大占用时间。
// 输出：成功返回幂等释放函数；已有未过期锁时返回 ErrAlreadyRunning。
// 副作用：写入并在释放时删除 SQLite job_execution_lock。
func acquireSQLiteLock(ctx context.Context, db *sql.DB, jobName string, ttl time.Duration) (func() error, error) {
	// 1. 校验锁名和期限，并创建不可猜测的本次执行令牌。
	lockName := sqliteJobLockPrefix + strings.TrimSpace(jobName)
	if lockName == sqliteJobLockPrefix || len(lockName) > 200 {
		return nil, fmt.Errorf("SQLite 任务锁名称无效: %q", jobName)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("任务 %s 的 SQLite 锁期限无效", jobName)
	}
	ownerToken, err := newLockToken()
	if err != nil {
		return nil, fmt.Errorf("创建任务 %s 锁令牌: %w", jobName, err)
	}

	// 2. 用单条 upsert 原子占用空锁或已过期锁。
	now := time.Now().UTC()
	expiresAt := now.Add(ttl + time.Minute)
	result, err := db.ExecContext(ctx, `
		INSERT INTO job_execution_lock(lock_name, owner_token, acquired_at, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(lock_name) DO UPDATE SET
			owner_token = excluded.owner_token,
			acquired_at = excluded.acquired_at,
			expires_at = excluded.expires_at
		WHERE job_execution_lock.expires_at <= excluded.acquired_at
	`, lockName, ownerToken, now.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("获取任务 %s 的 SQLite 锁: %w", jobName, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("读取任务 %s 的 SQLite 锁结果: %w", jobName, err)
	}
	if affected == 0 {
		return nil, fmt.Errorf("任务 %s: %w", jobName, ErrAlreadyRunning)
	}

	// 3. 返回只执行一次且只删除本次令牌的释放函数。
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := db.ExecContext(releaseContext,
				"DELETE FROM job_execution_lock WHERE lock_name = ? AND owner_token = ?",
				lockName, ownerToken)
			if err != nil {
				releaseErr = fmt.Errorf("释放任务 %s 的 SQLite 锁: %w", jobName, err)
				return
			}
			affected, err := result.RowsAffected()
			if err != nil {
				releaseErr = fmt.Errorf("读取任务 %s 的 SQLite 解锁结果: %w", jobName, err)
			} else if affected != 1 {
				releaseErr = fmt.Errorf("释放任务 %s 的 SQLite 锁未生效", jobName)
			}
		})
		return releaseErr
	}, nil
}

// newLockToken 创建任务锁所有者令牌。
// 输入：无。
// 输出：返回 128 位随机十六进制文本；系统随机源失败时返回错误。
// 副作用：读取操作系统安全随机源。
func newLockToken() (string, error) {
	// 1. 填充固定长度随机字节并编码为无分隔符文本。
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
