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
)

// TestNewArticleAnalysisModelsUsesOnlyDeepSeek 验证文章分析模型目录固定使用 DeepSeek。
func TestNewArticleAnalysisModelsUsesOnlyDeepSeek(t *testing.T) {
	models := newArticleAnalysisModels(config.Config{
		Clients: config.Clients{
			DeepSeek: config.DeepSeek{Model: "deepseek-v4-pro"},
		},
	})
	if len(models) != 1 {
		t.Fatalf("newArticleAnalysisModels() returned %d models, want 1", len(models))
	}
	if models[0].ID != "deepseek:deepseek-v4-pro" || models[0].Provider != "deepseek" || models[0].Model != "deepseek-v4-pro" {
		t.Fatalf("newArticleAnalysisModels() = %#v, want DeepSeek only", models[0])
	}
}

// TestRunShutsDownOnContextCancellation 验证 Run 会在上下文取消后优雅退出。
// 输入：开发 API 代理配置和可取消测试上下文。
// 输出：HTTP 服务启动后在十秒内无错误退出。
// 副作用：短暂监听本机端口。
func TestRunShutsDownOnContextCancellation(t *testing.T) {
	// 1. 使用无需数据库的开发代理模式启动运行时并等待 HTTP 监听器就绪。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	address := reserveAddress(t)
	go func() {
		// 2. 将运行结果交给测试协程。
		done <- Run(ctx, config.Config{
			Environment: "test",
			HTTP:        config.HTTP{Address: address, StaticDir: t.TempDir()},
			Development: config.Development{UpstreamURL: "http://127.0.0.1:1"},
		})
	}()
	waitForServer(t, address, done)

	// 3. 取消上下文并断言服务正常退出。
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
// 输入：缺少迁移目录的生产配置。
// 输出：Run 返回明确的迁移目录错误。
// 副作用：无，不打开数据库或监听端口。
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

// TestRunJobSkipsSchemaMigrations 验证 CLI 补跑不会解析 PostgreSQL 迁移目录。
// 输入：不可达 PostgreSQL 与故意不存在的生产迁移目录。
// 输出：返回连接错误而不是迁移目录错误。
// 副作用：尝试连接本机不可达端口。
func TestRunJobSkipsSchemaMigrations(t *testing.T) {
	// 1. 使用缺失迁移目录运行 CLI 任务，证明补跑路径先直接连接既有数据库。
	_, err := RunJob(context.Background(), config.Config{
		Environment:   "production",
		MigrationsDir: filepath.Join(t.TempDir(), "missing"),
		Database: config.Database{
			URL:          "postgres://invalid@127.0.0.1:1/invalid?sslmode=disable&connect_timeout=1",
			MaxOpenConns: 1, MaxIdleConns: 0, ConnMaxLifetime: time.Minute,
		},
		Storage: config.Storage{BackupDir: t.TempDir(), BackupRetention: 7},
	}, "test_crontab")
	if err == nil {
		t.Fatal("RunJob() error = nil, want PostgreSQL connection error")
	}
	if strings.Contains(err.Error(), "迁移目录") {
		t.Fatalf("RunJob() error = %v, should skip migrations", err)
	}
}

// TestResolveMigrationsDirectoryUsesProductionExecutableSibling 验证生产环境使用可执行文件同级迁移目录。
// 输入：带 migrations/sqlite 子目录的临时可执行文件路径。
// 输出：返回发布目录中的 SQLite 迁移路径。
// 副作用：创建测试临时目录。
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
// 输入：显式迁移目录和无效可执行文件路径。
// 输出：优先返回清理后的显式目录。
// 副作用：创建测试临时目录。
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
// 输入：不存在同级迁移目录的生产可执行文件路径。
// 输出：返回生产迁移目录缺失错误。
// 副作用：创建测试临时目录。
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
// 输入：开发环境、空显式目录和无效可执行文件路径。
// 输出：返回仓库中的 SQLite 迁移目录。
// 副作用：只读取源码目录元数据。
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
// 输入：t 接收监听失败报告。
// 输出：返回当前可用的回环地址。
// 副作用：短暂打开并关闭 TCP 监听器。
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
// 输入：t 接收失败，address 是目标地址，done 提前报告服务退出。
// 输出：服务可连接时返回；超时或提前退出时终止测试。
// 副作用：反复建立并关闭本机 TCP 连接。
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
