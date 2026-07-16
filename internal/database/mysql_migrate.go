package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
)

// MigrateMySQL 使用 Goose 按版本执行 MySQL 迁移。
// 输入：ctx 控制迁移生命周期，db 是目标 MySQL，directory 是只含 MySQL SQL 的目录。
// 输出：全部待执行版本成功后返回 nil，初始化或任一迁移失败时返回带上下文的错误。
// 副作用：创建或调整 MySQL 表、索引和 Goose 版本记录。
func MigrateMySQL(ctx context.Context, db *sql.DB, directory string) error {
	// 1. 用独立 Provider 绑定 MySQL 方言，避免 Goose 全局可变配置。
	provider, err := goose.NewProvider(goose.DialectMySQL, db, os.DirFS(directory))
	if err != nil {
		return fmt.Errorf("初始化 MySQL 迁移器: %w", err)
	}

	// 2. 按版本执行全部待应用迁移，已记录版本不会重复执行。
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("执行 MySQL 迁移: %w", err)
	}
	return nil
}
