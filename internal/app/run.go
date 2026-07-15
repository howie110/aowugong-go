package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	financejob "github.com/howiedata/aowugong-go/internal/finance/job"
	"github.com/howiedata/aowugong-go/internal/finance/position"
	financeservice "github.com/howiedata/aowugong-go/internal/finance/service"
	"github.com/howiedata/aowugong-go/internal/finance/stockanalysis"
	"github.com/howiedata/aowugong-go/internal/httpserver"
	"github.com/howiedata/aowugong-go/internal/mahjong"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/notification"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/weread"
	"github.com/howiedata/aowugong-go/internal/work"
)

const developmentJWTSecret = "aowugong-development-only-secret"

type appRuntime struct {
	db        *sql.DB
	handler   http.Handler
	registry  *scheduler.Registry
	scheduler *scheduler.CronScheduler
}

// Run 启动数据库迁移、内嵌调度器与 HTTP 服务，并在上下文取消时优雅关闭。
// 输入：ctx 控制服务生命周期，cfg 提供全部运行配置。
// 输出：正常关闭返回 nil，初始化、监听或关闭失败时返回带业务上下文的错误。
// 副作用：迁移并访问 SQLite、启动 HTTP/Cron，任务触发时访问外部服务和发送通知。
func Run(ctx context.Context, cfg config.Config) error {
	// 1. 组装所有显式依赖并在退出时关闭 SQLite。
	runtime, err := buildRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer runtime.db.Close()

	// 2. 按配置启动上海时区内嵌调度器。
	if cfg.Scheduler.Enabled {
		if err := runtime.scheduler.Start(); err != nil {
			return fmt.Errorf("启动内嵌调度器: %w", err)
		}
	}

	// 3. 启动 HTTP 服务并等待监听错误或根上下文取消。
	server := &http.Server{Addr: cfg.HTTP.Address, Handler: runtime.handler}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case serverErr := <-serverErrors:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if stopErr := runtime.scheduler.Stop(shutdownCtx); stopErr != nil {
			return stopErr
		}
		if errors.Is(serverErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("启动 HTTP 服务: %w", serverErr)
	case <-ctx.Done():
		// 4. 先停止新任务触发，再关闭 HTTP 并等待现有请求完成。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := runtime.scheduler.Stop(shutdownCtx); err != nil {
			return err
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务: %w", err)
		}
		return nil
	}
}

// RunJob 通过与自动和页面执行相同的注册表运行单个 CLI 任务。
// 输入：ctx 控制执行，cfg 提供依赖配置，name 是已注册任务名。
// 输出：返回统一执行结果；初始化或任务失败时返回错误。
// 副作用：迁移并访问 SQLite，执行目标任务，失败时可能发送微信通知。
func RunJob(ctx context.Context, cfg config.Config, name string) (scheduler.Result, error) {
	// 1. 组装相同运行时但不启动 HTTP 或 Cron。
	runtime, err := buildRuntime(ctx, cfg)
	if err != nil {
		return scheduler.Result{}, err
	}
	defer runtime.db.Close()

	// 2. 使用 CLI 来源调用唯一任务包装器。
	return runtime.registry.Run(ctx, name, scheduler.SourceCLI)
}

// buildRuntime 打开数据库、迁移表结构并显式构造全部服务。
// 输入：ctx 控制初始化，cfg 提供路径、密钥、外部地址和运行开关。
// 输出：返回 HTTP、任务和数据库运行时；失败时自动关闭已打开数据库。
// 副作用：迁移 SQLite、同步权限、默认账户和默认文章来源。
func buildRuntime(ctx context.Context, cfg config.Config) (*appRuntime, error) {
	// 1. 打开 SQLite 并应用发布产物或源码中的版本化迁移。
	db, err := database.OpenSQLite(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = db.Close()
		}
	}()
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取可执行文件路径: %w", err)
	}
	migrationsDirectory, err := resolveMigrationsDirectory(cfg.Environment, cfg.MigrationsDir, executablePath)
	if err != nil {
		return nil, fmt.Errorf("解析迁移目录: %w", err)
	}
	if err := database.Migrate(ctx, db, migrationsDirectory); err != nil {
		return nil, fmt.Errorf("迁移数据库: %w", err)
	}

	// 2. 构造认证、权限和普通业务服务并同步代码维护的基线。
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		return nil, fmt.Errorf("初始化角色权限: %w", err)
	}
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = developmentJWTSecret
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager(jwtSecret, cfg.Auth.TokenLifetime))
	subscriptionService := subscription.NewService(subscription.NewRepository(db))
	mahjongService := mahjong.NewService(mahjong.NewRepository(db))
	workService := work.NewService(cfg.Storage.WorkNavigationPath)
	httpClient := &http.Client{Timeout: 20 * time.Second}
	wereadService := weread.NewService(client.NewWeReadClient(cfg.Clients.WeRead, httpClient))
	monitoringService := monitoring.NewService(monitoring.NewRepository(db), client.NewMonitoringClient(httpClient), cfg.Clients)

	// 3. 构造 finance 页面、仓位、分析、文章、行情和统一通知服务。
	financeService := financeservice.NewDashboardService(db, financeservice.DashboardOptions{
		HTTPAddress: cfg.HTTP.Address, OpenILinkConfigured: cfg.Clients.OpenILink.AppToken != "",
		SchedulerEnabled: cfg.Scheduler.Enabled, RealTradeEnabled: cfg.Finance.EnableRealTrade,
		QMTConfigured: cfg.Finance.QMTAccount != "", BinanceConfigured: cfg.Finance.BinanceAPIKey != "",
		OKXConfigured: cfg.Finance.OKXAPIKey != "",
	})
	positionRepository := position.NewRepository(db)
	if err := positionRepository.SyncDefaultAccounts(ctx); err != nil {
		return nil, fmt.Errorf("初始化仓位账户: %w", err)
	}
	positionService := position.NewService(positionRepository, client.NewAliyunOCRClient(cfg.Clients.PositionOCR), position.UploadOptions{
		UploadDir: cfg.Storage.PositionUploadDir, TempDir: cfg.Storage.PositionTempDir,
		MaxBytes: cfg.Clients.PositionOCR.UploadMaxMB * 1024 * 1024, OCRProvider: cfg.Clients.PositionOCR.Provider,
	})
	stockAnalysisService := stockanalysis.NewService(stockanalysis.NewRepository(db))
	articleRepository := articleanalysis.NewRepository(db)
	if err := articleRepository.SyncDefaultSource(ctx, cfg.Clients.ArticleRSSURL); err != nil {
		return nil, fmt.Errorf("初始化投资文章来源: %w", err)
	}
	articleService := articleanalysis.NewService(articleRepository, articleanalysis.ServiceOptions{
		Model: cfg.Clients.DeepSeek.Model, RSS: client.NewRSSClient(&http.Client{Timeout: 180 * time.Second}),
		Analyzer: client.NewDeepSeekClient(cfg.Clients.DeepSeek, &http.Client{Timeout: 60 * time.Second}),
	})
	dataService := financedata.NewService(financedata.NewRepository(db), client.NewTushareClient(cfg.Clients.Tushare, nil), financedata.SyncOptions{
		LookbackDays: 60, Delay: time.Second,
	})
	notificationService := notification.NewService(notification.NewRepository(db), client.NewOpenILinkClient(cfg.Clients.OpenILink, nil))

	// 4. 建立七项任务的唯一注册表和 Asia/Shanghai Cron 调度器。
	jobRegistry := scheduler.NewRegistry(db, notificationService, slog.Default())
	backupDir, backupRetention := cfg.Storage.BackupDir, cfg.Storage.BackupRetention
	if backupDir == "" {
		backupDir = "storage/backup"
	}
	if backupRetention < 1 {
		backupRetention = 7
	}
	if err := financejob.RegisterAll(jobRegistry, financejob.Dependencies{
		DB: db, Data: dataService, Articles: articleService, Monitoring: monitoringService,
		Subscriptions: subscriptionService, Notification: notificationService,
		BackupDir: backupDir, BackupRetention: backupRetention,
	}); err != nil {
		return nil, fmt.Errorf("注册生产任务: %w", err)
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, fmt.Errorf("加载调度时区 Asia/Shanghai: %w", err)
	}
	cronScheduler := scheduler.NewCronScheduler(jobRegistry, location)

	// 5. 把同一业务服务和任务注册表交给 HTTP 路由。
	handler := httpserver.NewRouter(httpserver.Dependencies{
		StaticDir: cfg.HTTP.StaticDir, Auth: authService, RBAC: rbacService,
		Subscription: subscriptionService, Mahjong: mahjongService, Work: workService,
		WeRead: wereadService, Monitoring: monitoringService, Finance: financeService,
		Position: positionService, StockAnalysis: stockAnalysisService, ArticleAnalysis: articleService,
		Jobs: jobRegistry,
	})
	success = true
	return &appRuntime{db: db, handler: handler, registry: jobRegistry, scheduler: cronScheduler}, nil
}
