package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// serveRuntime 只运行现有 HTTP 监听器，通知和其他 API 一起由 Caddy 提供 HTTPS。
// 先绑定端口再启动调度器，关闭时给在途通知留出写回发送结果的时间。
func serveRuntime(ctx context.Context, cfg config.Config, runtime *appRuntime) error {
	server := &http.Server{Addr: cfg.HTTP.Address, Handler: runtime.handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("监听 HTTP: %w", err)
	}
	defer listener.Close()
	defer server.Close()
	if cfg.Scheduler.Enabled && runtime.scheduler != nil {
		if err := runtime.scheduler.Start(); err != nil {
			return fmt.Errorf("启动内嵌调度器: %w", err)
		}
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	var result error
	select {
	case <-ctx.Done():
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			result = fmt.Errorf("HTTP 服务退出: %w", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 28*time.Second)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(shutdownCtx) }()
	if runtime.scheduler != nil {
		result = errors.Join(result, runtime.scheduler.Stop(shutdownCtx))
	}
	return errors.Join(result, <-shutdownDone)
}
