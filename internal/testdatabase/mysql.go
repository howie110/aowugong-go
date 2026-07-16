// Package testdatabase 提供隔离 MySQL 集成测试库。
package testdatabase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

var schemaCounter atomic.Uint64

// Prepare 创建当前测试独享的空 MySQL schema 并返回连接配置。
// 输入：t 管理缺少环境时跳过、失败报告和测试结束清理。
// 输出：返回只指向新 schema 的 MySQL 配置。
// 副作用：连接测试 MySQL，创建并在测试结束时删除临时 schema。
func Prepare(t *testing.T) config.Database {
	// 1. 读取专用测试账号，任何字段缺失都跳过而不回退生产配置。
	t.Helper()
	base := loadBaseConfig(t)
	admin, err := database.OpenMySQL(context.Background(), base)
	if err != nil {
		t.Fatalf("打开 MySQL 测试管理连接: %v", err)
	}

	// 2. 使用进程和递增编号生成只含安全字符的隔离 schema。
	schemaName := fmt.Sprintf("aowugong_go_test_%d_%d", os.Getpid(), schemaCounter.Add(1))
	if _, err := admin.ExecContext(context.Background(), "CREATE DATABASE `"+schemaName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("创建 MySQL 测试 schema: %v", err)
	}
	t.Cleanup(func() {
		// 3. 测试连接关闭后删除临时 schema，不保留测试业务数据。
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+schemaName+"`"); err != nil {
			t.Errorf("删除 MySQL 测试 schema: %v", err)
		}
		_ = admin.Close()
	})
	base.Name = schemaName
	return base
}

// Open 创建、迁移并打开当前测试独享的 MySQL schema。
// 输入：t 管理测试生命周期。
// 输出：返回已经包含全部运行时表的数据库连接池。
// 副作用：创建、迁移并在测试结束时删除临时 MySQL schema。
func Open(t *testing.T) *sql.DB {
	// 1. 创建隔离 schema 并建立目标连接。
	t.Helper()
	cfg := Prepare(t)
	db, err := database.OpenMySQL(context.Background(), cfg)
	if err != nil {
		t.Fatalf("打开隔离 MySQL 测试库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 2. 使用仓库中的正式 MySQL 迁移建立测试结构。
	if err := database.MigrateMySQL(context.Background(), db, MigrationsDirectory(t)); err != nil {
		t.Fatalf("迁移隔离 MySQL 测试库: %v", err)
	}
	return db
}

// MigrationsDirectory 返回仓库中的正式 MySQL 迁移目录。
// 输入：t 接收无法定位源码时的失败报告。
// 输出：返回绝对目录路径。
// 副作用：无，只读取当前源码文件位置。
func MigrationsDirectory(t *testing.T) string {
	// 1. 从当前测试辅助包源码稳定定位仓库根目录。
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 MySQL 测试迁移目录")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations", "mysql"))
}

// loadBaseConfig 读取可创建隔离 schema 的 MySQL 测试账号。
// 输入：t 管理缺失配置和格式错误。
// 输出：返回连接固定基础测试库的小型连接池配置。
// 副作用：读取 AOWUGONG_TEST_MYSQL_* 环境变量。
func loadBaseConfig(t *testing.T) config.Database {
	// 1. 收集全部必需变量，缺少任一项就跳过集成测试。
	t.Helper()
	keys := []string{
		"AOWUGONG_TEST_MYSQL_HOST", "AOWUGONG_TEST_MYSQL_PORT", "AOWUGONG_TEST_MYSQL_DATABASE",
		"AOWUGONG_TEST_MYSQL_USER", "AOWUGONG_TEST_MYSQL_PASSWORD",
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value := os.Getenv(key)
		if value == "" {
			t.Skipf("%s is not configured", key)
		}
		values[key] = value
	}

	// 2. 校验端口并建立固定的小连接池配置。
	port, err := strconv.Atoi(values["AOWUGONG_TEST_MYSQL_PORT"])
	if err != nil {
		t.Fatalf("解析 MySQL 测试端口: %v", err)
	}
	return config.Database{
		Host: values["AOWUGONG_TEST_MYSQL_HOST"], Port: port,
		Name: values["AOWUGONG_TEST_MYSQL_DATABASE"], User: values["AOWUGONG_TEST_MYSQL_USER"],
		Password: values["AOWUGONG_TEST_MYSQL_PASSWORD"], MaxOpenConns: 4, MaxIdleConns: 1,
		ConnMaxLifetime: time.Minute,
	}
}
