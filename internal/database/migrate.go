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
		return fmt.Errorf("read migrations: %w", err)
	}

	// 2. 在单个事务内创建记录表并执行未应用迁移。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		var applied int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", entry.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if applied != 0 {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return fmt.Errorf("migration %s is empty", entry.Name())
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename) VALUES (?)", entry.Name()); err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
	}

	// 3. 提交包含所有迁移的事务。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
