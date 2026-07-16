package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestRunShutsDownOnContextCancellation 验证 Run 会在上下文取消后优雅退出。
func TestRunShutsDownOnContextCancellation(t *testing.T) {
	// 1. 创建当前测试独享的 MySQL schema。
	databaseConfig := testdatabase.Prepare(t)

	// 2. 启动运行时并等待 HTTP 监听器就绪。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	address := reserveAddress(t)
	migrationsDirectory := testdatabase.MigrationsDirectory(t)
	go func() {
		// 3. 将运行结果交给测试协程。
		done <- Run(ctx, config.Config{
			Environment:   "test",
			MigrationsDir: migrationsDirectory,
			HTTP:          config.HTTP{Address: address},
			Database:      databaseConfig,
		})
	}()
	waitForServer(t, address, done)

	// 4. 取消上下文并断言服务正常退出。
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

// TestRunRejectsMissingProductionMigrations 验证生产环境不会回退到编译时源码迁移目录。
func TestRunRejectsMissingProductionMigrations(t *testing.T) {
	// 1. 使用无显式迁移目录的生产配置启动应用。
	err := Run(context.Background(), config.Config{
		Environment: "production",
		HTTP:        config.HTTP{Address: "invalid"},
	})

	// 2. 断言启动在迁移阶段返回明确中文错误。
	if err == nil {
		t.Fatal("Run() error = nil, want missing production migrations error")
	}
	if !strings.Contains(err.Error(), "生产环境缺少迁移目录") {
		t.Errorf("Run() error = %q, want production migrations error", err)
	}
}

// TestRunJobSkipsSchemaMigrations 验证 CLI 补跑只使用既有 MySQL 表结构。
// 输入：已迁移的隔离 MySQL 与故意不存在的生产迁移目录。
// 输出：任务正常完成，不因迁移目录缺失失败。
// 副作用：创建隔离 schema，并写入一条 test_crontab 执行记录。
func TestRunJobSkipsSchemaMigrations(t *testing.T) {
	// 1. 先由测试管理账号建立完整表结构，模拟已部署的生产库。
	databaseConfig := testdatabase.Prepare(t)
	db, err := database.OpenMySQL(context.Background(), databaseConfig)
	if err != nil {
		t.Fatalf("database.OpenMySQL() error = %v", err)
	}
	if err := database.MigrateMySQL(context.Background(), db, testdatabase.MigrationsDirectory(t)); err != nil {
		_ = db.Close()
		t.Fatalf("database.MigrateMySQL() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}

	// 2. 使用缺失迁移目录运行 CLI 任务，证明补跑路径不会执行 DDL。
	result, err := RunJob(context.Background(), config.Config{
		Environment:   "production",
		MigrationsDir: filepath.Join(t.TempDir(), "missing"),
		Database:      databaseConfig,
		Storage:       config.Storage{BackupDir: t.TempDir(), BackupRetention: 7},
	}, "test_crontab")
	if err != nil {
		t.Fatalf("RunJob() error = %v", err)
	}
	if result.Status != "success" {
		t.Errorf("RunJob() status = %q, want success", result.Status)
	}
}

// TestResolveMigrationsDirectoryUsesProductionExecutableSibling 验证生产环境使用可执行文件同级迁移目录。
func TestResolveMigrationsDirectoryUsesProductionExecutableSibling(t *testing.T) {
	// 1. 创建模拟生产压缩包中的可执行文件同级迁移目录。
	executableDirectory := t.TempDir()
	want := filepath.Join(executableDirectory, migrationDirectoryName)
	if err := os.MkdirAll(want, 0o750); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// 2. 解析默认迁移目录并断言生产目录优先。
	got, err := resolveMigrationsDirectory("production", "", filepath.Join(executableDirectory, "aowugong.exe"))
	if err != nil {
		t.Fatalf("resolveMigrationsDirectory() error = %v", err)
	}
	if got != want {
		t.Errorf("resolveMigrationsDirectory() = %q, want %q", got, want)
	}
}

// TestResolveMigrationsDirectoryUsesProductionOverride 验证生产环境可使用显式迁移目录。
func TestResolveMigrationsDirectoryUsesProductionOverride(t *testing.T) {
	// 1. 创建显式迁移目录和模拟生产迁移目录。
	overrideDirectory := filepath.Join(t.TempDir(), "custom-migrations")
	if err := os.Mkdir(overrideDirectory, 0o750); err != nil {
		t.Fatalf("os.Mkdir() override error = %v", err)
	}
	executableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(executableDirectory, migrationDirectoryName), 0o750); err != nil {
		t.Fatalf("os.MkdirAll() executable sibling error = %v", err)
	}

	// 2. 解析迁移目录并断言显式配置优先。
	got, err := resolveMigrationsDirectory("production", overrideDirectory, filepath.Join(executableDirectory, "aowugong.exe"))
	if err != nil {
		t.Fatalf("resolveMigrationsDirectory() error = %v", err)
	}
	if got != overrideDirectory {
		t.Errorf("resolveMigrationsDirectory() = %q, want %q", got, overrideDirectory)
	}
}

// TestResolveMigrationsDirectoryRejectsMissingProductionDirectory 验证生产环境缺少发布迁移目录时返回明确错误。
func TestResolveMigrationsDirectoryRejectsMissingProductionDirectory(t *testing.T) {
	// 1. 使用没有同级迁移目录的模拟生产可执行文件。
	executablePath := filepath.Join(t.TempDir(), "aowugong.exe")

	// 2. 断言解析失败且错误说明生产迁移目录缺失。
	got, err := resolveMigrationsDirectory("production", "", executablePath)
	if err == nil {
		t.Fatal("resolveMigrationsDirectory() error = nil, want missing production migrations error")
	}
	if got != "" {
		t.Errorf("resolveMigrationsDirectory() = %q, want empty path", got)
	}
	if !strings.Contains(err.Error(), "生产环境缺少迁移目录") {
		t.Errorf("resolveMigrationsDirectory() error = %q, want production migrations error", err)
	}
}

// TestResolveMigrationsDirectoryUsesSourceFallbackOutsideProduction 验证开发和测试环境允许源码目录回退。
func TestResolveMigrationsDirectoryUsesSourceFallbackOutsideProduction(t *testing.T) {
	// 1. 计算测试源码对应的仓库根迁移目录。
	want, err := filepath.Abs(filepath.Join("..", "..", migrationDirectoryName))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	// 2. 断言开发和测试环境均回退到源码迁移目录。
	for _, environment := range []string{"development", "test"} {
		got, err := resolveMigrationsDirectory(environment, "", filepath.Join(t.TempDir(), "aowugong.exe"))
		if err != nil {
			t.Fatalf("resolveMigrationsDirectory(%q) error = %v", environment, err)
		}
		if got != want {
			t.Errorf("resolveMigrationsDirectory(%q) = %q, want %q", environment, got, want)
		}
	}
}

// reserveAddress 预留一个本地 TCP 地址供测试使用。
func reserveAddress(t *testing.T) string {
	// 1. 让操作系统分配临时端口并立即释放监听器。
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close() error = %v", err)
	}
	return address
}

// waitForServer 等待测试服务开始接受 TCP 连接。
func waitForServer(t *testing.T, address string, done <-chan error) {
	// 1. 在超时前轮询服务监听状态。
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		select {
		case runErr := <-done:
			t.Fatalf("Run() returned before listening: %v", runErr)
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	t.Fatal("server did not start listening")
}
