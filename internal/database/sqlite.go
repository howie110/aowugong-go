// Package database 提供 SQLite 运行时支持。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/howiedata/aowugong-go/internal/config"
	_ "modernc.org/sqlite"
)

// OpenSQLite 打开并配置应用使用的 SQLite 数据库。
func OpenSQLite(ctx context.Context, cfg config.Database) (*sql.DB, error) {
	// 1. 创建文件数据库所需的父目录。
	path := filepath.Clean(cfg.Path)
	if path == "." || path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	// 2. 限制连接池为单一写入连接并建立连接。
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// 3. 在唯一连接上启用运行时 pragma。
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}

	return db, nil
}
