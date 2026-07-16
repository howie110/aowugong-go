package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestMigrateMySQLCreatesRuntimeTables 验证全新 MySQL 库可迁移且重复执行无副作用。
// 输入：AOWUGONG_TEST_MYSQL_* 指向专用空测试库。
// 输出：两次迁移均成功，并存在任务记录、用户和行情表。
// 副作用：在专用 MySQL 测试库创建表和迁移记录，未配置测试库时跳过。
func TestMigrateMySQLCreatesRuntimeTables(t *testing.T) {
	// 1. 打开显式提供的隔离 MySQL 测试库。
	db := openMySQLIntegrationTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	directory := filepath.Join("..", "..", "migrations", "mysql")

	// 2. 连续执行两次迁移，验证首次创建与幂等重跑。
	if err := MigrateMySQL(ctx, db, directory); err != nil {
		t.Fatalf("first MigrateMySQL() error = %v", err)
	}
	if err := MigrateMySQL(ctx, db, directory); err != nil {
		t.Fatalf("second MigrateMySQL() error = %v", err)
	}

	// 3. 核对运行时依赖的关键表已经存在。
	for _, tableName := range []string{"job_execution", "notification_log", "aowugong_fastapi_users", "tushare_daily"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("query table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Errorf("table %s count = %d, want 1", tableName, count)
		}
	}
}

// openMySQLIntegrationTest 打开环境变量指定的专用 MySQL 测试库。
// 输入：t 管理跳过、失败和连接清理。
// 输出：返回已验证连通性的 MySQL 连接池。
// 副作用：建立网络连接；缺少任一测试变量时跳过当前测试。
func openMySQLIntegrationTest(t *testing.T) *sql.DB {
	// 1. 读取完整测试身份，禁止回退到生产默认值。
	t.Helper()
	required := []string{
		"AOWUGONG_TEST_MYSQL_HOST", "AOWUGONG_TEST_MYSQL_PORT", "AOWUGONG_TEST_MYSQL_DATABASE",
		"AOWUGONG_TEST_MYSQL_USER", "AOWUGONG_TEST_MYSQL_PASSWORD",
	}
	values := make(map[string]string, len(required))
	for _, key := range required {
		value := os.Getenv(key)
		if value == "" {
			t.Skipf("%s is not configured", key)
		}
		values[key] = value
	}
	port, err := strconv.Atoi(values["AOWUGONG_TEST_MYSQL_PORT"])
	if err != nil {
		t.Fatalf("parse test MySQL port: %v", err)
	}

	// 2. 使用小连接池打开测试库并注册清理。
	db, err := OpenMySQL(context.Background(), config.Database{
		Host: values["AOWUGONG_TEST_MYSQL_HOST"], Port: port,
		Name: values["AOWUGONG_TEST_MYSQL_DATABASE"], User: values["AOWUGONG_TEST_MYSQL_USER"],
		Password: values["AOWUGONG_TEST_MYSQL_PASSWORD"], MaxOpenConns: 4, MaxIdleConns: 1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("OpenMySQL() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
