package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
)

// MigratePostgres 使用 Goose 按版本执行 PostgreSQL 迁移。
// 输入：ctx 控制迁移生命周期，db 是目标 PostgreSQL，directory 是迁移 SQL 目录。
// 输出：全部待执行版本成功后返回 nil。
// 副作用：创建或调整 PostgreSQL 表、索引和 Goose 版本记录。
func MigratePostgres(ctx context.Context, db *sql.DB, directory string) error {
	// 1. 使用独立 Provider 绑定 PostgreSQL 方言。
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(directory))
	if err != nil {
		return fmt.Errorf("初始化 PostgreSQL 迁移器: %w", err)
	}

	// 2. 幂等执行全部待应用迁移。
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("执行 PostgreSQL 迁移: %w", err)
	}
	return nil
}
