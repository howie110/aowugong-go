package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Migrate 按文件名顺序在单个事务内执行未应用的 SQL 迁移。
func Migrate(ctx context.Context, db *sql.DB, dir string) error {
	// 1. 读取并验证迁移目录。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取迁移目录: %w", err)
	}

	// 2. 在单个事务内创建记录表并执行未应用迁移。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始迁移事务: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("创建迁移记录表: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		var applied int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", entry.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("检查迁移 %s: %w", entry.Name(), err)
		}
		if applied != 0 {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("读取迁移 %s: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return fmt.Errorf("迁移 %s 为空", entry.Name())
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("执行迁移 %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename) VALUES (?)", entry.Name()); err != nil {
			return fmt.Errorf("记录迁移 %s: %w", entry.Name(), err)
		}
	}

	// 3. 提交包含所有迁移的事务。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移: %w", err)
	}
	return nil
}
