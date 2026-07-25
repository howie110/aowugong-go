package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestMigrateSQLiteCreatesRuntimeTables 验证全新 SQLite 可迁移且重复执行无副作用。
// 输入：测试临时数据库和正式 SQLite 迁移目录。
// 输出：两次迁移均成功，并存在任务、用户、行情和锁表。
// 副作用：在测试临时目录创建 SQLite 文件和全部表。
func TestMigrateSQLiteCreatesRuntimeTables(t *testing.T) {
	// 1. 打开当前测试独享的 SQLite 文件。
	ctx := context.Background()
	db, err := OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "migration.db"), MaxOpenConns: 4,
		MaxIdleConns: 1, BusyTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	directory := filepath.Join("..", "..", "migrations", "sqlite")

	// 2. 连续执行两次迁移，验证首次创建与幂等重跑。
	if err := MigrateSQLite(ctx, db, directory); err != nil {
		t.Fatalf("first MigrateSQLite() error = %v", err)
	}
	if err := MigrateSQLite(ctx, db, directory); err != nil {
		t.Fatalf("second MigrateSQLite() error = %v", err)
	}

	// 3. 核对运行时依赖的关键表已经存在。
	for _, tableName := range []string{
		"job_execution", "job_execution_lock", "notification_log",
		"aowugong_fastapi_users", "tushare_daily",
		"investment_signal_group", "investment_signal_alias",
	} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema
			WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("query table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Errorf("table %s count = %d, want 1", tableName, count)
		}
	}

	// 4. 核对证券行业初始词典已经落库。
	var seededAliases int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM investment_signal_alias alias
		JOIN investment_signal_group signal_group ON signal_group.id = alias.group_id
		WHERE signal_group.canonical_name = '证券行业'
		  AND alias.alias_name IN ('券商', '券商板块', '证券板块', '中信证券')
	`).Scan(&seededAliases); err != nil {
		t.Fatalf("query seeded securities aliases: %v", err)
	}
	if seededAliases != 4 {
		t.Errorf("seeded securities aliases = %d, want 4", seededAliases)
	}
}

// TestSignalGroupMigrationDeclaresRequiredSecuritiesAliases 验证迁移包含证券行业初始词典。
// 输入：SQLite 基线迁移文件。
// 输出：迁移声明证券行业及四个固定别名。
// 副作用：只读本地迁移文件。
func TestSignalGroupMigrationDeclaresRequiredSecuritiesAliases(t *testing.T) {
	// 1. 读取负责创建概念词典的版本化迁移。
	path := filepath.Join("..", "..", "migrations", "sqlite", "00001_baseline.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read signal group migration: %v", err)
	}

	// 2. 明确业务规则必须由迁移确定，不能只依赖模型临场判断。
	text := string(content)
	for _, value := range []string{"证券行业", "券商", "券商板块", "证券板块", "中信证券"} {
		if !strings.Contains(text, value) {
			t.Errorf("migration is missing %q", value)
		}
	}
}

// TestSQLiteDefaultsUseShanghaiWallClock 验证 SQLite 新记录默认时间保持现有上海时间语义。
// 输入：正式 baseline 创建的认证用户表。
// 输出：数据库默认 created_at 与当前 UTC+8 墙上时间接近。
// 副作用：创建临时 SQLite 并写入一条测试用户。
func TestSQLiteDefaultsUseShanghaiWallClock(t *testing.T) {
	// 1. 打开并迁移隔离 SQLite。
	ctx := context.Background()
	db, err := OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "timezone.db"), MaxOpenConns: 1,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := MigrateSQLite(ctx, db, filepath.Join("..", "..", "migrations", "sqlite")); err != nil {
		t.Fatalf("MigrateSQLite() error = %v", err)
	}

	// 2. 省略时间字段写入并核对默认值固定为 UTC+8。
	if _, err := db.ExecContext(ctx, `
		INSERT INTO aowugong_fastapi_users(username, email, password)
		VALUES('timezone-user', 'timezone@example.com', 'hash')
	`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var createdAtText string
	if err := db.QueryRowContext(ctx, `
		SELECT created_at FROM aowugong_fastapi_users WHERE username = 'timezone-user'
	`).Scan(&createdAtText); err != nil {
		t.Fatalf("query created_at: %v", err)
	}
	createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtText)
	if err != nil {
		t.Fatalf("parse created_at %q: %v", createdAtText, err)
	}
	expected := time.Now().UTC().Add(8 * time.Hour)
	if difference := expected.Sub(createdAt); difference < -3*time.Second || difference > 3*time.Second {
		t.Errorf("created_at = %s, want UTC+8 near %s", createdAtText, expected.Format("2006-01-02 15:04:05"))
	}
}
