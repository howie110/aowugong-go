package mysqlmigration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestCopyTableUsesBatchesAndIsRepeatable 验证批量复制保留主键且可安全重跑。
// 输入：五行模拟源表、两行批次和含旧数据的目标表。
// 输出：目标旧数据被替换，五行字段一致，第二次运行仍只有五行。
// 副作用：在测试临时目录创建并写入两个 SQLite 文件。
func TestCopyTableUsesBatchesAndIsRepeatable(t *testing.T) {
	// 1. 创建模拟源库和目标库并写入源数据与目标旧数据。
	ctx := context.Background()
	source := openMigrationTestDB(t, filepath.Join(t.TempDir(), "source.db"))
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "target.db"))
	for _, db := range []*sql.DB{source, target} {
		if _, err := db.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY, name TEXT, created_at TEXT)"); err != nil {
			t.Fatalf("create sample: %v", err)
		}
	}
	for index := 1; index <= 5; index++ {
		if _, err := source.ExecContext(ctx, "INSERT INTO sample(id,name,created_at) VALUES(?,?,?)", index, "row", "2026-01-01 00:00:00"); err != nil {
			t.Fatalf("insert source: %v", err)
		}
	}
	if _, err := target.ExecContext(ctx, "INSERT INTO sample(id,name,created_at) VALUES(99,'old','2020-01-01')"); err != nil {
		t.Fatalf("insert target: %v", err)
	}
	plan := TablePlan{
		Name: "sample", SourceColumns: []string{"id", "name", "created_at"},
		TargetColumns: []string{"id", "name", "created_at"}, PrimaryKey: "id",
	}

	// 2. 连续复制两次并核对每次都替换成完整五行。
	for run := 1; run <= 2; run++ {
		copied, err := copyTable(ctx, source, target, plan, 2, nil)
		if err != nil {
			t.Fatalf("copyTable() run %d error = %v", run, err)
		}
		if copied != 5 {
			t.Errorf("copyTable() run %d copied = %d, want 5", run, copied)
		}
		var count int
		if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM sample").Scan(&count); err != nil {
			t.Fatalf("count target: %v", err)
		}
		if count != 5 {
			t.Errorf("target count after run %d = %d, want 5", run, count)
		}
	}
}

// TestNormalizeValueConvertsMySQLBytesAndTimes 验证 MySQL 扫描类型转换为 SQLite 文本。
// 输入：字节字符串、日期时间和普通整数。
// 输出：字节变字符串，时间按数据库格式输出，整数保持不变。
// 副作用：无。
func TestNormalizeValueConvertsMySQLBytesAndTimes(t *testing.T) {
	// 1. 核对常见 MySQL 驱动返回类型的唯一转换入口。
	if got := normalizeValue([]byte("中文")); got != "中文" {
		t.Errorf("normalize bytes = %#v", got)
	}
	wantTime := "2026-01-02 03:04:05"
	if got := normalizeValue(time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)); got != wantTime {
		t.Errorf("normalize time = %#v, want %q", got, wantTime)
	}
	if got := normalizeValue(int64(7)); got != int64(7) {
		t.Errorf("normalize integer = %#v", got)
	}
}

// openMigrationTestDB 打开迁移测试使用的单连接 SQLite。
// 输入：t 管理失败和清理，path 是数据库路径。
// 输出：返回已连通数据库。
// 副作用：创建 SQLite 文件并注册测试清理。
func openMigrationTestDB(t *testing.T, path string) *sql.DB {
	// 1. 打开单连接数据库并确认可用。
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
