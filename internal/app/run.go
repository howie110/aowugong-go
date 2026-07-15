package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/httpserver"
)

// Run 启动数据库迁移与 HTTP 服务，并在上下文取消时优雅关闭。
func Run(ctx context.Context, cfg config.Config) error {
	// 1. 打开数据库并应用运行时迁移。
	db, err := database.OpenSQLite(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, migrationDirectory); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	// 2. 启动 HTTP 服务并等待服务错误或取消信号。
	server := &http.Server{
		Addr:    cfg.HTTP.Address,
		Handler: httpserver.NewRouter(httpserver.Dependencies{StaticDir: cfg.HTTP.StaticDir}),
	}
	serverErrors := make(chan error, 1)
	go func() {
		// 3. 将服务退出结果发送给主运行流程。
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
		// 4. 使用独立超时上下文完成优雅关闭。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http: %w", err)
		}
		return nil
	}
}
