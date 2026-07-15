package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/httpserver"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

const developmentJWTSecret = "aowugong-development-only-secret"

// Run 启动数据库迁移与 HTTP 服务，并在上下文取消时优雅关闭。
func Run(ctx context.Context, cfg config.Config) error {
	// 1. 打开数据库并应用运行时迁移。
	db, err := database.OpenSQLite(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("打开数据库: %w", err)
	}
	defer db.Close()
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径: %w", err)
	}
	migrationsDirectory, err := resolveMigrationsDirectory(cfg.Environment, cfg.MigrationsDir, executablePath)
	if err != nil {
		return fmt.Errorf("解析迁移目录: %w", err)
	}
	if err := database.Migrate(ctx, db, migrationsDirectory); err != nil {
		return fmt.Errorf("迁移数据库: %w", err)
	}

	// 2. 显式组装认证和权限服务，并同步代码维护的权限基线。
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		return fmt.Errorf("初始化角色权限: %w", err)
	}
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = developmentJWTSecret
	}
	authService := auth.NewService(
		auth.NewRepository(db),
		auth.NewTokenManager(jwtSecret, cfg.Auth.TokenLifetime),
	)

	// 3. 启动 HTTP 服务并等待服务错误或取消信号。
	server := &http.Server{
		Addr: cfg.HTTP.Address,
		Handler: httpserver.NewRouter(httpserver.Dependencies{
			StaticDir: cfg.HTTP.StaticDir,
			Auth:      authService,
			RBAC:      rbacService,
		}),
	}
	serverErrors := make(chan error, 1)
	go func() {
		// 4. 将服务退出结果发送给主运行流程。
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("启动 HTTP 服务: %w", err)
	case <-ctx.Done():
		// 5. 使用独立超时上下文完成优雅关闭。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务: %w", err)
		}
		return nil
	}
}
