package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
)

// MigrateSQLite 使用 Goose 按版本执行 SQLite 迁移。
// 输入：ctx 控制迁移生命周期，db 是目标 SQLite，directory 是 SQLite SQL 目录。
// 输出：全部待执行版本成功后返回 nil，初始化或执行失败时返回错误。
// 副作用：创建或调整 SQLite 表、索引和 Goose 版本记录。
func MigrateSQLite(ctx context.Context, db *sql.DB, directory string) error {
	// 1. 使用独立 Provider 绑定 SQLite 方言，避免 Goose 全局状态。
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS(directory))
	if err != nil {
		return fmt.Errorf("初始化 SQLite 迁移器: %w", err)
	}

	// 2. 幂等执行全部待应用版本。
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("执行 SQLite 迁移: %w", err)
	}
	return nil
}
