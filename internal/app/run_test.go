package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestRunShutsDownOnContextCancellation 验证 Run 会在上下文取消后优雅退出。
func TestRunShutsDownOnContextCancellation(t *testing.T) {
	// 1. 切换至包含迁移目录的仓库根目录并准备运行配置。
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}
	// 2. 在测试结束时恢复工作目录。
	defer os.Chdir(oldWorkingDir)

	// 3. 启动运行时并等待 HTTP 监听器就绪。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	address := reserveAddress(t)
	go func() {
		// 4. 将运行结果交给测试协程。
		done <- Run(ctx, config.Config{
			HTTP:     config.HTTP{Address: address},
			Database: config.Database{Path: filepath.Join(t.TempDir(), "runtime.db")},
		})
	}()
	waitForServer(t, address, done)

	// 5. 取消上下文并断言服务正常退出。
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
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
	deadline := time.Now().Add(3 * time.Second)
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
