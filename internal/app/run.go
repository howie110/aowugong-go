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
	"github.com/howiedata/aowugong-go/internal/databaseview"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	financedata "github.com/howiedata/aowugong-go/internal/finance/data"
	financejob "github.com/howiedata/aowugong-go/internal/finance/job"
	"github.com/howiedata/aowugong-go/internal/finance/position"
	financeservice "github.com/howiedata/aowugong-go/internal/finance/service"
	"github.com/howiedata/aowugong-go/internal/finance/stockanalysis"
	"github.com/howiedata/aowugong-go/internal/githubbackup"
	"github.com/howiedata/aowugong-go/internal/httpserver"
	"github.com/howiedata/aowugong-go/internal/mahjong"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/notification"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/vaultwardenbackup"
	"github.com/howiedata/aowugong-go/internal/vpn"
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

type jobRuntime struct {
	db       *sql.DB
	registry *scheduler.Registry
}

type taskServices struct {
	subscriptions     *subscription.Service
	monitoring        *monitoring.Service
	articleRepository *articleanalysis.Repository
	articles          *articleanalysis.Service
	data              *financedata.Service
	notification      *notification.Service
	githubBackup      *githubbackup.Service
	vaultwardenBackup *vaultwardenbackup.Service
}

// Run 启动数据库迁移、内嵌调度器与 HTTP 服务，并在上下文取消时优雅关闭。
// 输入：ctx 控制服务生命周期，cfg 提供全部运行配置。
// 输出：正常关闭返回 nil，初始化、监听或关闭失败时返回带业务上下文的错误。
// 副作用：迁移并访问 PostgreSQL、启动 HTTP/Cron，任务触发时访问外部服务和发送通知。
func Run(ctx context.Context, cfg config.Config) error {
	// 1. 组装所有显式依赖并在退出时关闭 PostgreSQL 连接池。
	runtime, err := buildRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	if runtime.db != nil {
		defer runtime.db.Close()
	}

	// 2. 按配置启动上海时区内嵌调度器。
	if cfg.Scheduler.Enabled && runtime.scheduler != nil {
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
		if runtime.scheduler != nil {
			if stopErr := runtime.scheduler.Stop(shutdownCtx); stopErr != nil {
				return stopErr
			}
		}
		if errors.Is(serverErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("启动 HTTP 服务: %w", serverErr)
	case <-ctx.Done():
		// 4. 先停止新任务触发，再关闭 HTTP 并等待现有请求完成。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if runtime.scheduler != nil {
			if err := runtime.scheduler.Stop(shutdownCtx); err != nil {
				return err
			}
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
// 副作用：访问既有 PostgreSQL 表结构并执行目标任务，失败时可能发送微信通知；不执行数据库迁移。
func RunJob(ctx context.Context, cfg config.Config, name string) (scheduler.Result, error) {
	// 1. 组装只含任务依赖的运行时，不启动 HTTP、Cron 或数据库迁移。
	runtime, err := buildJobRuntime(ctx, cfg)
	if err != nil {
		return scheduler.Result{}, err
	}
	defer runtime.db.Close()

	// 2. 使用 CLI 来源调用唯一任务包装器。
	return runtime.registry.Run(ctx, name, scheduler.SourceCLI)
}

// buildJobRuntime 打开既有 PostgreSQL 并组装服务器补跑所需的最小任务运行时。
// 输入：ctx 控制连接，cfg 提供数据库、外部客户端和备份配置。
// 输出：返回数据库与统一任务注册表；连接或注册失败时返回错误。
// 副作用：连接 PostgreSQL；不迁移表结构、不写默认数据、不启动 HTTP 或 Cron。
func buildJobRuntime(ctx context.Context, cfg config.Config) (*jobRuntime, error) {
	// 1. 仅连接由服务器部署流程维护好结构的 PostgreSQL。
	db, err := database.OpenPostgres(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("打开任务数据库: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = db.Close()
		}
	}()

	// 2. 构造任务服务并注册自动调度、页面手动和 CLI 共用的全部任务。
	services := newTaskServices(cfg, db)
	registry, err := newJobRegistry(cfg, db, services)
	if err != nil {
		return nil, err
	}
	success = true
	return &jobRuntime{db: db, registry: registry}, nil
}

// buildRuntime 打开数据库、迁移表结构并显式构造全部服务。
// 输入：ctx 控制初始化，cfg 提供路径、密钥、外部地址和运行开关。
// 输出：返回 HTTP、任务和数据库运行时；失败时自动关闭已打开数据库。
// 副作用：迁移 PostgreSQL、同步权限、默认账户和默认文章来源。
func buildRuntime(ctx context.Context, cfg config.Config) (*appRuntime, error) {
	// 1. 开发上游模式只服务本地前端并代理线上 API，不创建本地数据库。
	if cfg.Development.UpstreamURL != "" {
		handler, err := httpserver.NewDevelopmentRouter(cfg.HTTP.StaticDir, cfg.Development.UpstreamURL)
		if err != nil {
			return nil, fmt.Errorf("创建开发 API 代理: %w", err)
		}
		return &appRuntime{handler: handler}, nil
	}

	// 2. 需要迁移时先解析目录，避免配置错误发生后才连接生产数据库。
	migrationsDirectory := ""
	if !cfg.Database.SkipMigrations {
		executablePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("获取可执行文件路径: %w", err)
		}
		migrationsDirectory, err = resolveMigrationsDirectory(cfg.Environment, cfg.MigrationsDir, executablePath)
		if err != nil {
			return nil, fmt.Errorf("解析迁移目录: %w", err)
		}
	}

	// 3. 打开 PostgreSQL 小型连接池，失败时确保释放已建立连接。
	db, err := database.OpenPostgres(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = db.Close()
		}
	}()

	// 4. 服务器默认应用版本迁移，CLI 任务入口显式跳过 DDL。
	if !cfg.Database.SkipMigrations {
		if err := database.MigratePostgres(ctx, db, migrationsDirectory); err != nil {
			return nil, fmt.Errorf("迁移数据库: %w", err)
		}
	}

	// 5. 构造认证、权限和普通业务服务并同步代码维护的基线。
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		return nil, fmt.Errorf("初始化角色权限: %w", err)
	}
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = developmentJWTSecret
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager(jwtSecret, cfg.Auth.TokenLifetime))
	mahjongService := mahjong.NewService(mahjong.NewRepository(db))
	workService := work.NewService(cfg.Storage.WorkNavigationPath)
	httpClient := &http.Client{Timeout: 20 * time.Second}
	wereadService := weread.NewService(client.NewWeReadClient(cfg.Clients.WeRead, httpClient))
	tasks := newTaskServices(cfg, db)

	// 6. 构造 finance 页面、仓位、分析、文章、行情和统一通知服务。
	financeService := financeservice.NewDashboardService(db, financeservice.DashboardOptions{
		HTTPAddress: cfg.HTTP.Address, OpenILinkConfigured: cfg.Clients.OpenILink.AppToken != "",
		SchedulerEnabled: cfg.Scheduler.Enabled, GitHubBackupEnabled: cfg.GitHubBackup.Enabled,
		VaultwardenBackupEnabled: cfg.VaultwardenBackup.Enabled, RealTradeEnabled: cfg.Finance.EnableRealTrade,
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
	vpnSecret := cfg.Auth.EncryptionKey
	if vpnSecret == "" {
		vpnSecret = developmentJWTSecret
	}
	vpnService := vpn.NewService(
		vpn.NewRepository(db),
		vpn.NewSourceCatalog(cfg.VPN.SourceDir),
		vpn.NewDirectDistributor(cfg.VPN.PublicURL),
		vpnSecret,
	)
	if err := tasks.articleRepository.SyncWeReadSource(ctx); err != nil {
		return nil, fmt.Errorf("初始化投资文章来源: %w", err)
	}

	// 7. 建立定时与手动任务的唯一注册表和 Asia/Shanghai Cron 调度器。
	jobRegistry, err := newJobRegistry(cfg, db, tasks)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, fmt.Errorf("加载调度时区 Asia/Shanghai: %w", err)
	}
	cronScheduler := scheduler.NewCronScheduler(jobRegistry, location)

	// 8. 把同一业务服务和任务注册表交给 HTTP 路由。
	handler := httpserver.NewRouter(httpserver.Dependencies{
		StaticDir: cfg.HTTP.StaticDir, Auth: authService, RBAC: rbacService,
		Subscription: tasks.subscriptions, Mahjong: mahjongService, Work: workService,
		WeRead: wereadService, Monitoring: tasks.monitoring, Finance: financeService,
		Position: positionService, StockAnalysis: stockAnalysisService, ArticleAnalysis: tasks.articles,
		Jobs: jobRegistry, Database: databaseview.NewService(db),
		VPN: vpnService,
	})
	success = true
	return &appRuntime{db: db, handler: handler, registry: jobRegistry, scheduler: cronScheduler}, nil
}

// newTaskServices 构造自动调度、页面手动执行和 CLI 补跑共用的业务服务。
// 输入：cfg 提供外部接口参数，db 是应用 PostgreSQL 连接池。
// 输出：返回订阅、监控、文章、行情和通知服务集合。
// 副作用：无，只构造依赖；不会访问数据库或外部接口。
func newTaskServices(cfg config.Config, db *sql.DB) taskServices {
	// 1. 构造普通业务、监控和微信读书文章来源，并保留仓储供服务器初始化。
	articleRepository := articleanalysis.NewRepository(db)
	encryptionSecret := cfg.Auth.EncryptionKey
	if encryptionSecret == "" {
		encryptionSecret = developmentJWTSecret
	}
	weReadSource := articleanalysis.NewWeReadSource(
		articleRepository,
		client.NewWeReadArticleClient(&http.Client{Timeout: 30 * time.Second}),
		encryptionSecret,
	)
	services := taskServices{
		subscriptions:     subscription.NewService(subscription.NewRepository(db)),
		monitoring:        monitoring.NewService(monitoring.NewRepository(db), client.NewMonitoringClient(&http.Client{Timeout: 20 * time.Second}), cfg.Clients),
		articleRepository: articleRepository,
		articles: articleanalysis.NewService(articleRepository, articleanalysis.ServiceOptions{
			Model: cfg.Clients.DeepSeek.Model, Articles: weReadSource, WeRead: weReadSource,
			Analyzer: client.NewDeepSeekClient(cfg.Clients.DeepSeek, &http.Client{Timeout: 60 * time.Second}),
		}),
		data: financedata.NewService(financedata.NewRepository(db), client.NewTushareClient(cfg.Clients.Tushare, nil), financedata.SyncOptions{
			LookbackDays: 60, Delay: time.Second,
		}),
		notification: notification.NewService(notification.NewRepository(db), client.NewOpenILinkClient(cfg.Clients.OpenILink, nil)),
	}
	if cfg.GitHubBackup.Enabled {
		services.githubBackup = githubbackup.NewService(
			githubbackup.NewClient(cfg.GitHubBackup.Token, &http.Client{Timeout: 30 * time.Second}),
			cfg.GitHubBackup.RequiredRepositories,
			githubbackup.NewGitStore(cfg.GitHubBackup.Token),
			githubbackup.Options{
				Directory: cfg.GitHubBackup.Directory, RetentionRefs: cfg.GitHubBackup.RetentionRefs,
			},
		)
	}
	if cfg.VaultwardenBackup.Enabled {
		services.vaultwardenBackup = vaultwardenbackup.NewService(
			client.NewEmailClient(cfg.Clients.Email),
			vaultwardenbackup.Options{
				Directory:                cfg.VaultwardenBackup.Directory,
				RecoveryScriptsDirectory: cfg.VaultwardenBackup.RecoveryScriptsDirectory,
				AgeRecipient:             cfg.VaultwardenBackup.AgeRecipient,
				EmailTo:                  cfg.VaultwardenBackup.EmailTo,
				MaxAttachmentBytes:       int64(cfg.VaultwardenBackup.MaxAttachmentMB) * 1024 * 1024,
			},
		)
	}
	return services
}

// newJobRegistry 建立全部执行来源共用的任务注册表。
// 输入：cfg 提供数据库备份参数，db 写执行记录，services 提供业务入口。
// 输出：返回已注册完整定义的 Registry；依赖无效时返回错误。
// 副作用：只修改新建注册表的内存定义，不执行任务。
func newJobRegistry(cfg config.Config, db *sql.DB, services taskServices) (*scheduler.Registry, error) {
	// 1. 对未经过 Config.Load 的测试配置补齐备份默认值。
	backupDir, backupRetention := cfg.Storage.BackupDir, cfg.Storage.BackupRetention
	if backupDir == "" {
		backupDir = "storage/backup"
	}
	if backupRetention < 1 {
		backupRetention = 7
	}

	// 2. 把唯一业务服务交给任务定义，Registry 统一负责锁、记录和失败通知。
	registry := scheduler.NewRegistry(db, services.notification, slog.Default())
	dependencies := financejob.Dependencies{
		DB: db, Data: services.data, Articles: services.articles,
		Monitoring: services.monitoring, Subscriptions: services.subscriptions, Notification: services.notification,
		BackupDir: backupDir, BackupRetention: backupRetention,
		Backup: database.NewPostgresBackuper(cfg.Database.URL).Backup,
	}
	// 3. 指针先判空再赋给接口，避免 typed nil 被误判为已启用的可选任务。
	if services.githubBackup != nil {
		dependencies.GitHubBackup = services.githubBackup
	}
	if services.vaultwardenBackup != nil {
		dependencies.VaultwardenBackup = services.vaultwardenBackup
	}
	if err := financejob.RegisterAll(registry, dependencies); err != nil {
		return nil, fmt.Errorf("注册生产任务: %w", err)
	}
	return registry, nil
}
