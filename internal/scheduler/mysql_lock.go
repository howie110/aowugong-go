package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

const mysqlJobLockPrefix = "aowugong:job:"

// acquireMySQLLock 尝试获取绑定到独立 MySQL 会话的任务互斥锁。
// 输入：ctx 控制获取连接和无等待加锁，db 提供连接池，jobName 是规范化任务名。
// 输出：成功返回可幂等调用的释放函数；锁被占用时返回 ErrAlreadyRunning。
// 副作用：独占一条 MySQL 连接直到释放，并调用 GET_LOCK 和 RELEASE_LOCK。
func acquireMySQLLock(ctx context.Context, db *sql.DB, jobName string) (func() error, error) {
	// 1. 构造受 MySQL 64 字节限制约束的全局任务锁名。
	lockName := mysqlJobLockPrefix + strings.TrimSpace(jobName)
	if strings.TrimSpace(jobName) == "" || len(lockName) > 64 {
		return nil, fmt.Errorf("MySQL 任务锁名称无效: %q", jobName)
	}

	// 2. 独占一个连接并立即尝试获取会话级建议锁。
	connection, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取任务 %s 的 MySQL 锁连接: %w", jobName, err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockName).Scan(&acquired); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("获取任务 %s 的 MySQL 锁: %w", jobName, err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		return nil, fmt.Errorf("任务 %s: %w", jobName, ErrAlreadyRunning)
	}

	// 3. 返回只执行一次的释放函数，确保 RELEASE_LOCK 使用同一连接。
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var released sql.NullInt64
			if err := connection.QueryRowContext(releaseContext, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
				releaseErr = fmt.Errorf("释放任务 %s 的 MySQL 锁: %w", jobName, err)
			} else if !released.Valid || released.Int64 != 1 {
				releaseErr = fmt.Errorf("释放任务 %s 的 MySQL 锁未生效", jobName)
			}
			if err := connection.Close(); err != nil && releaseErr == nil {
				releaseErr = fmt.Errorf("归还任务 %s 的 MySQL 锁连接: %w", jobName, err)
			}
		})
		return releaseErr
	}, nil
}
