// Package testdatabase 提供隔离 SQLite 测试库。
package testdatabase

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Open 创建、迁移并打开当前测试独享的 SQLite 文件。
// 输入：t 管理测试生命周期。
// 输出：返回已经包含全部运行时表的数据库连接池。
// 副作用：创建、迁移并在测试结束时删除临时 SQLite 文件。
func Open(t *testing.T) *sql.DB {
	// 1. 创建隔离文件，并把测试所需的 SQLite 参数写入连接地址。
	t.Helper()
	path := filepath.Join(t.TempDir(), "aowugong-test.db")
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		urlPath = "/" + urlPath
	}
	fileURL := url.URL{Scheme: "file", Path: urlPath}
	query := fileURL.Query()
	for _, pragma := range []string{
		"foreign_keys(1)", "journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)",
	} {
		query.Add("_pragma", pragma)
	}
	fileURL.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", fileURL.String())
	if err != nil {
		t.Fatalf("打开隔离 SQLite 测试库: %v", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("连接隔离 SQLite 测试库: %v", err)
	}

	// 2. 仅在测试夹具中使用旧 SQLite 迁移建立隔离结构。
	provider, err := goose.NewProvider(
		goose.DialectSQLite3, db, os.DirFS(MigrationsDirectory(t)),
	)
	if err != nil {
		t.Fatalf("初始化隔离 SQLite 迁移器: %v", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("迁移隔离 SQLite 测试库: %v", err)
	}
	return db
}

// MigrationsDirectory 返回仓库中的测试 SQLite 迁移目录。
// 输入：t 接收无法定位源码时的失败报告。
// 输出：返回绝对目录路径。
// 副作用：无，只读取当前源码文件位置。
func MigrationsDirectory(t *testing.T) string {
	// 1. 从当前测试辅助包源码稳定定位仓库根目录。
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 SQLite 测试迁移目录")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations", "sqlite"))
}
