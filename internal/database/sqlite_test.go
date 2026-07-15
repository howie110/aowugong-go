package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestOpenSQLiteConfiguresRuntime 验证 SQLite 运行时连接参数。
func TestOpenSQLiteConfiguresRuntime(t *testing.T) {
	// 1. 打开临时文件数据库。
	db := openTestSQLite(t, filepath.Join(t.TempDir(), "runtime.db"))
	defer db.Close()

	// 2. 断言四个 pragma 的实际值。
	assertPragmaString(t, db, "journal_mode", "wal")
	assertPragmaInteger(t, db, "foreign_keys", 1)
	assertPragmaInteger(t, db, "busy_timeout", 5000)
	assertPragmaInteger(t, db, "synchronous", 1)

	// 3. 断言数据库只允许一个写入连接。
	if stats := db.Stats(); stats.MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}

// TestOpenSQLiteCreatesStorageDirectory 验证 SQLite 会创建缺失的存储目录。
func TestOpenSQLiteCreatesStorageDirectory(t *testing.T) {
	// 1. 指定尚不存在的嵌套数据库路径。
	path := filepath.Join(t.TempDir(), "storage", "nested", "runtime.db")

	// 2. 打开数据库并断言文件已建立。
	db := openTestSQLite(t, path)
	defer db.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("os.Stat(%q) error = %v", path, err)
	}
}

// TestMigrateCreatesJobExecution 验证迁移创建 job_execution 表。
func TestMigrateCreatesJobExecution(t *testing.T) {
	// 1. 打开数据库并执行仓库中的运行时迁移。
	db := openTestSQLite(t, filepath.Join(t.TempDir(), "runtime.db"))
	defer db.Close()
	if err := Migrate(context.Background(), db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// 2. 断言 job_execution 表已经存在。
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'job_execution'").Scan(&tableName)
	if err != nil {
		t.Fatalf("job_execution lookup error = %v", err)
	}
	if tableName != "job_execution" {
		t.Errorf("table name = %q, want job_execution", tableName)
	}
}

// TestMigrateRollsBackFailedBatch 验证失败迁移会回滚整个批次。
func TestMigrateRollsBackFailedBatch(t *testing.T) {
	// 1. 创建包含有效和无效 SQL 的迁移目录。
	dir := t.TempDir()
	writeMigration(t, dir, "00001_first.sql", "CREATE TABLE rolled_back (id INTEGER PRIMARY KEY);")
	writeMigration(t, dir, "00002_broken.sql", "CREATE TABLE broken (")
	db := openTestSQLite(t, filepath.Join(t.TempDir(), "runtime.db"))
	defer db.Close()

	// 2. 执行迁移并断言其失败。
	if err := Migrate(context.Background(), db, dir); err == nil {
		t.Fatal("Migrate() error = nil, want invalid SQL error")
	}

	// 3. 断言有效迁移创建的表也被事务回滚。
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'rolled_back'").Scan(&count); err != nil {
		t.Fatalf("rolled_back lookup error = %v", err)
	}
	if count != 0 {
		t.Errorf("rolled_back table count = %d, want 0", count)
	}
}

// openTestSQLite 打开供数据库测试使用的 SQLite 实例。
func openTestSQLite(t *testing.T, path string) *sql.DB {
	// 1. 使用真实运行时入口建立数据库。
	t.Helper()
	db, err := OpenSQLite(context.Background(), config.Database{Path: path})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	return db
}

// assertPragmaString 断言字符串类型 pragma 的返回值。
func assertPragmaString(t *testing.T, db *sql.DB, name, want string) {
	// 1. 查询 pragma 并比较字符串值。
	t.Helper()
	var got string
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("PRAGMA %s error = %v", name, err)
	}
	if got != want {
		t.Errorf("PRAGMA %s = %q, want %q", name, got, want)
	}
}

// assertPragmaInteger 断言整数类型 pragma 的返回值。
func assertPragmaInteger(t *testing.T, db *sql.DB, name string, want int) {
	// 1. 查询 pragma 并比较整数值。
	t.Helper()
	var got int
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("PRAGMA %s error = %v", name, err)
	}
	if got != want {
		t.Errorf("PRAGMA %s = %d, want %d", name, got, want)
	}
}

// writeMigration 写入供迁移测试使用的 SQL 文件。
func writeMigration(t *testing.T, dir, name, content string) {
	// 1. 将指定内容写入迁移目录。
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", name, err)
	}
}
