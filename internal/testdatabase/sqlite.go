// Package testdatabase 提供隔离 SQLite 测试库。
package testdatabase

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// Prepare 创建当前测试独享的 SQLite 文件配置。
// 输入：t 管理临时目录生命周期。
// 输出：返回指向新临时文件的 SQLite 配置。
// 副作用：创建测试临时目录，不创建数据库文件。
func Prepare(t *testing.T) config.Database {
	// 1. 使用 testing 临时目录保证并行测试互不共享数据库。
	t.Helper()
	return config.Database{
		Path:         filepath.Join(t.TempDir(), "aowugong-test.db"),
		MaxOpenConns: 4, MaxIdleConns: 1, BusyTimeout: 5 * time.Second,
	}
}

// Open 创建、迁移并打开当前测试独享的 SQLite 文件。
// 输入：t 管理测试生命周期。
// 输出：返回已经包含全部运行时表的数据库连接池。
// 副作用：创建、迁移并在测试结束时删除临时 SQLite 文件。
func Open(t *testing.T) *sql.DB {
	// 1. 创建隔离配置并打开 SQLite。
	t.Helper()
	cfg := Prepare(t)
	db, err := database.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("打开隔离 SQLite 测试库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 2. 使用正式 SQLite 迁移建立测试结构。
	if err := database.MigrateSQLite(context.Background(), db, MigrationsDirectory(t)); err != nil {
		t.Fatalf("迁移隔离 SQLite 测试库: %v", err)
	}
	return db
}

// MigrationsDirectory 返回仓库中的正式 SQLite 迁移目录。
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
