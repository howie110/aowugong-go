package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/datamigration"
)

// main 校验显式确认后执行 MySQL 到 SQLite 的一次性迁移。
// 输入：读取 --confirm 参数和环境变量。
// 输出：成功时输出 JSON 核对报告；失败时以非零状态退出。
// 副作用：只读旧 MySQL，迁移并重写目标 SQLite。
func main() {
	// 1. 把完整执行交给可测试入口并统一处理致命错误。
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// run 加载迁移配置、建立连接并输出迁移报告。
// 输入：args 必须包含 --confirm。
// 输出：迁移和核对成功返回 nil，否则返回带上下文的错误。
// 副作用：创建 SQLite 文件、执行版本迁移并复制 MySQL 数据。
func run(args []string) error {
	// 1. 要求显式确认，避免误清空已有 SQLite 目标数据。
	if len(args) != 2 || args[1] != "--confirm" {
		return fmt.Errorf("用法: migrate --confirm")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("加载迁移配置: %w", err)
	}
	if cfg.Migration.MySQL.Password == "" {
		return fmt.Errorf("迁移必须配置 AOWUGONG_MYSQL_PASSWORD")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. 打开旧 MySQL 和目标 SQLite，并先建立最终表结构。
	source, err := datamigration.OpenMySQLSource(ctx, cfg.Migration.MySQL)
	if err != nil {
		return fmt.Errorf("打开迁移来源: %w", err)
	}
	defer source.Close()
	target, err := database.OpenSQLite(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("打开迁移目标: %w", err)
	}
	defer target.Close()
	migrationsDirectory, err := resolveMigrationsDirectory(cfg.MigrationsDir)
	if err != nil {
		return err
	}
	if err := database.MigrateSQLite(ctx, target, migrationsDirectory); err != nil {
		return fmt.Errorf("建立 SQLite 结构: %w", err)
	}

	// 3. 执行原子复制并把最终核对报告写到标准输出。
	report, err := datamigration.MySQLToSQLite(ctx, source, target, os.Stderr)
	if err != nil {
		return fmt.Errorf("迁移 MySQL 到 SQLite: %w", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return fmt.Errorf("输出迁移报告: %w", err)
	}
	return nil
}

// resolveMigrationsDirectory 定位发布包或源码中的 SQLite 迁移目录。
// 输入：configured 是可选显式目录。
// 输出：返回存在的目录；无法定位时返回错误。
// 副作用：读取文件系统元数据。
func resolveMigrationsDirectory(configured string) (string, error) {
	// 1. 显式配置优先。
	if configured != "" {
		return filepath.Clean(configured), nil
	}

	// 2. 发布环境优先读取迁移工具同级目录。
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位迁移工具: %w", err)
	}
	releaseDirectory := filepath.Join(filepath.Dir(executable), "migrations", "sqlite")
	if info, err := os.Stat(releaseDirectory); err == nil && info.IsDir() {
		return releaseDirectory, nil
	}

	// 3. 源码运行时从当前文件定位仓库目录。
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位 SQLite 迁移目录")
	}
	sourceDirectory := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations", "sqlite"))
	if info, err := os.Stat(sourceDirectory); err != nil || !info.IsDir() {
		return "", fmt.Errorf("SQLite 迁移目录不存在: %s", sourceDirectory)
	}
	return sourceDirectory, nil
}
