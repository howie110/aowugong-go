// Package config 负责加载运行时配置。
package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvironment        = "development"
	defaultHTTPAddress        = "0.0.0.0:2345"
	defaultSQLitePath         = "storage/data/aowugong.db"
	defaultSQLiteMaxOpenConns = 4
	defaultSQLiteMaxIdleConns = 2
	defaultSQLiteBusyTimeout  = 5 * time.Second
	defaultMySQLHost          = "127.0.0.1"
	defaultMySQLPort          = 3306
	defaultMySQLDatabase      = "aowugong"
	defaultMySQLUser          = "aowugong"
	defaultStaticDir          = "web/dist"
	defaultTokenLifetime      = 72 * time.Hour
	defaultWorkNavigationPath = "storage/private/work/navigation.json"
	defaultBackupDir          = "storage/backup"
	defaultPositionUploadDir  = "storage/uploads/positions"
	defaultPositionTempDir    = "storage/temp/positions"
)

// LookupEnv 定义查询环境变量的函数类型。
type LookupEnv func(string) (string, bool)

// Config 汇总应用运行所需的配置。
type Config struct {
	Environment   string
	MigrationsDir string
	HTTP          HTTP
	Database      Database
	Migration     Migration
	Development   Development
	Auth          Auth
	Storage       Storage
	Clients       Clients
	Finance       Finance
	Scheduler     Scheduler
}

// HTTP 描述 HTTP 服务配置。
type HTTP struct {
	Address   string
	StaticDir string
}

// Database 描述 SQLite 文件、锁等待和连接池配置。
type Database struct {
	Path           string
	MaxOpenConns   int
	MaxIdleConns   int
	BusyTimeout    time.Duration
	SkipMigrations bool
}

// Migration 描述一次性数据迁移所需的旧 MySQL 来源。
type Migration struct {
	MySQL MySQLSource
}

// MySQLSource 描述只供 cmd/migrate 使用的旧 MySQL 连接。
type MySQLSource struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// Development 描述本地前端复用线上 API 的开发模式。
type Development struct {
	UpstreamURL string
}

// Auth 描述令牌与加密配置。
type Auth struct {
	JWTSecret     string
	EncryptionKey string
	TokenLifetime time.Duration
}

// Storage 描述运行时文件位置和 SQLite 备份保留策略。
type Storage struct {
	WorkNavigationPath string
	BackupDir          string
	BackupRetention    int
	PositionUploadDir  string
	PositionTempDir    string
}

// Clients 描述当前可达业务使用的外部 HTTP 服务配置。
type Clients struct {
	WeRead                WeRead
	Miniflux              Miniflux
	DeepSeek              DeepSeek
	Tushare               Tushare
	OpenILink             OpenILink
	ServiceMonitorTargets string
	PositionOCR           PositionOCR
}

// Miniflux 描述投资文章聚合 API 配置。
type Miniflux struct {
	BaseURL    string
	MonitorURL string
	APIToken   string
	Category   string
}

// WeRead 描述微信读书 Agent Gateway 配置。
type WeRead struct {
	GatewayURL   string
	APIKey       string
	SkillVersion string
}

// DeepSeek 描述投资文章结构化分析客户端配置。
type DeepSeek struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Tushare 描述 Tushare HTTP API 配置。
type Tushare struct {
	BaseURL string
	Token   string
}

// OpenILink 描述微信通知和链路监控配置。
type OpenILink struct {
	HubURL     string
	MonitorURL string
	AppToken   string
	DefaultTo  string
	DBPath     string
}

// PositionOCR 描述仓位截图 OCR 配置。
type PositionOCR struct {
	Provider        string
	UploadMaxMB     int
	AccessKeyID     string
	AccessKeySecret string
	Endpoint        string
}

// Finance 描述交易保护和现有交易端配置状态。
type Finance struct {
	EnableRealTrade bool
	QMTAccount      string
	BinanceAPIKey   string
	OKXAPIKey       string
}

// Scheduler 描述内嵌任务调度器开关。
type Scheduler struct {
	Enabled bool
}

// Load 从环境变量加载并校验应用配置。
// 输入：lookup 按名称读取进程环境变量。
// 输出：返回已规范化配置；格式、范围或生产密钥无效时返回错误。
// 副作用：无，不修改进程环境。
func Load(lookup LookupEnv) (Config, error) {
	// 1. 建立所有运行环境共用的默认配置。
	cfg := Config{
		Environment: defaultEnvironment,
		HTTP: HTTP{
			Address:   defaultHTTPAddress,
			StaticDir: defaultStaticDir,
		},
		Database: Database{
			Path: defaultSQLitePath, MaxOpenConns: defaultSQLiteMaxOpenConns,
			MaxIdleConns: defaultSQLiteMaxIdleConns, BusyTimeout: defaultSQLiteBusyTimeout,
		},
		Migration: Migration{
			MySQL: MySQLSource{
				Host: defaultMySQLHost, Port: defaultMySQLPort,
				Database: defaultMySQLDatabase, User: defaultMySQLUser,
			},
		},
		Auth: Auth{
			TokenLifetime: defaultTokenLifetime,
		},
		Storage: Storage{
			WorkNavigationPath: defaultWorkNavigationPath,
			BackupDir:          defaultBackupDir,
			BackupRetention:    7,
			PositionUploadDir:  defaultPositionUploadDir,
			PositionTempDir:    defaultPositionTempDir,
		},
		Clients: Clients{
			WeRead: WeRead{
				GatewayURL:   "https://i.weread.qq.com/api/agent/gateway",
				SkillVersion: "1.0.4",
			},
			Miniflux: Miniflux{
				BaseURL: "http://127.0.0.1:5000", MonitorURL: "http://8.138.123.59:5000/", Category: "投资文章",
			},
			DeepSeek: DeepSeek{BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-pro"},
			Tushare:  Tushare{BaseURL: "https://api.tushare.pro"},
			OpenILink: OpenILink{
				HubURL:     "http://127.0.0.1:9800",
				MonitorURL: "http://8.138.123.59:9800/",
				DBPath:     "/var/lib/openilink-hub/openilink.db",
			},
			PositionOCR: PositionOCR{
				Provider:    "aliyun",
				UploadMaxMB: 10,
				Endpoint:    "ocr-api.cn-hangzhou.aliyuncs.com",
			},
		},
	}

	// 2. 使用非空环境变量覆盖默认配置。
	if value, ok := lookup("AOWUGONG_ENV"); ok && strings.TrimSpace(value) != "" {
		cfg.Environment = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_HTTP_ADDRESS"); ok && strings.TrimSpace(value) != "" {
		cfg.HTTP.Address = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_STATIC_DIR"); ok && strings.TrimSpace(value) != "" {
		cfg.HTTP.StaticDir = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_MIGRATIONS_DIR"); ok && strings.TrimSpace(value) != "" {
		cfg.MigrationsDir = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_JWT_SECRET"); ok {
		cfg.Auth.JWTSecret = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_ENCRYPTION_KEY"); ok {
		cfg.Auth.EncryptionKey = strings.TrimSpace(value)
	}
	loadString(lookup, "AOWUGONG_WORK_NAVIGATION_PATH", &cfg.Storage.WorkNavigationPath)
	loadString(lookup, "AOWUGONG_SQLITE_PATH", &cfg.Database.Path)
	loadString(lookup, "AOWUGONG_DEV_UPSTREAM_URL", &cfg.Development.UpstreamURL)
	loadString(lookup, "AOWUGONG_MYSQL_HOST", &cfg.Migration.MySQL.Host)
	loadString(lookup, "AOWUGONG_MYSQL_DATABASE", &cfg.Migration.MySQL.Database)
	loadString(lookup, "AOWUGONG_MYSQL_USER", &cfg.Migration.MySQL.User)
	loadString(lookup, "AOWUGONG_MYSQL_PASSWORD", &cfg.Migration.MySQL.Password)
	loadString(lookup, "AOWUGONG_BACKUP_DIR", &cfg.Storage.BackupDir)
	loadString(lookup, "AOWUGONG_POSITION_UPLOAD_DIR", &cfg.Storage.PositionUploadDir)
	loadString(lookup, "AOWUGONG_POSITION_TEMP_DIR", &cfg.Storage.PositionTempDir)
	loadString(lookup, "WEREAD_GATEWAY_URL", &cfg.Clients.WeRead.GatewayURL)
	loadString(lookup, "WEREAD_API_KEY", &cfg.Clients.WeRead.APIKey)
	loadString(lookup, "WEREAD_SKILL_VERSION", &cfg.Clients.WeRead.SkillVersion)
	loadString(lookup, "MINIFLUX_BASE_URL", &cfg.Clients.Miniflux.BaseURL)
	loadString(lookup, "MINIFLUX_MONITOR_URL", &cfg.Clients.Miniflux.MonitorURL)
	loadString(lookup, "MINIFLUX_API_TOKEN", &cfg.Clients.Miniflux.APIToken)
	loadString(lookup, "MINIFLUX_CATEGORY", &cfg.Clients.Miniflux.Category)
	loadString(lookup, "DEEPSEEK_BASE_URL", &cfg.Clients.DeepSeek.BaseURL)
	loadString(lookup, "DEEPSEEK_API_KEY", &cfg.Clients.DeepSeek.APIKey)
	loadString(lookup, "DEEPSEEK_MODEL", &cfg.Clients.DeepSeek.Model)
	loadString(lookup, "TUSHARE_BASE_URL", &cfg.Clients.Tushare.BaseURL)
	loadString(lookup, "TUSHARE_TOKEN", &cfg.Clients.Tushare.Token)
	loadString(lookup, "OPENILINK_HUB_URL", &cfg.Clients.OpenILink.HubURL)
	loadString(lookup, "OPENILINK_MONITOR_URL", &cfg.Clients.OpenILink.MonitorURL)
	loadString(lookup, "OPENILINK_APP_TOKEN", &cfg.Clients.OpenILink.AppToken)
	loadString(lookup, "OPENILINK_DEFAULT_TO", &cfg.Clients.OpenILink.DefaultTo)
	loadString(lookup, "OPENILINK_DB_PATH", &cfg.Clients.OpenILink.DBPath)
	loadString(lookup, "SERVICE_MONITOR_TARGETS", &cfg.Clients.ServiceMonitorTargets)
	loadString(lookup, "POSITION_OCR_PROVIDER", &cfg.Clients.PositionOCR.Provider)
	loadString(lookup, "ALIYUN_OCR_ACCESS_KEY_ID", &cfg.Clients.PositionOCR.AccessKeyID)
	loadString(lookup, "ALIYUN_OCR_ACCESS_KEY_SECRET", &cfg.Clients.PositionOCR.AccessKeySecret)
	loadString(lookup, "ALIYUN_OCR_ENDPOINT", &cfg.Clients.PositionOCR.Endpoint)
	loadString(lookup, "QMT_ACCOUNT", &cfg.Finance.QMTAccount)
	loadString(lookup, "BINANCE_API_KEY", &cfg.Finance.BinanceAPIKey)
	loadString(lookup, "OKX_API_KEY", &cfg.Finance.OKXAPIKey)
	if err := loadInt(lookup, "AOWUGONG_BACKUP_RETENTION", &cfg.Storage.BackupRetention, 1, 365); err != nil {
		return Config{}, err
	}
	if err := loadInt(lookup, "AOWUGONG_MYSQL_PORT", &cfg.Migration.MySQL.Port, 1, 65535); err != nil {
		return Config{}, err
	}
	if err := loadInt(lookup, "AOWUGONG_SQLITE_MAX_OPEN_CONNS", &cfg.Database.MaxOpenConns, 1, 32); err != nil {
		return Config{}, err
	}
	if err := loadInt(lookup, "AOWUGONG_SQLITE_MAX_IDLE_CONNS", &cfg.Database.MaxIdleConns, 0, 32); err != nil {
		return Config{}, err
	}
	busyTimeoutMS := int(cfg.Database.BusyTimeout / time.Millisecond)
	if err := loadInt(lookup, "AOWUGONG_SQLITE_BUSY_TIMEOUT_MS", &busyTimeoutMS, 100, 60000); err != nil {
		return Config{}, err
	}
	cfg.Database.BusyTimeout = time.Duration(busyTimeoutMS) * time.Millisecond
	if err := loadInt(lookup, "POSITION_UPLOAD_MAX_MB", &cfg.Clients.PositionOCR.UploadMaxMB, 1, 100); err != nil {
		return Config{}, err
	}
	if err := loadBool(lookup, "FINANCE_ENABLE_REAL_TRADE", &cfg.Finance.EnableRealTrade); err != nil {
		return Config{}, err
	}
	if err := loadBool(lookup, "AOWUGONG_SCHEDULER_ENABLED", &cfg.Scheduler.Enabled); err != nil {
		return Config{}, err
	}
	if err := loadBool(lookup, "AOWUGONG_SQLITE_SKIP_MIGRATIONS", &cfg.Database.SkipMigrations); err != nil {
		return Config{}, err
	}

	// 3. 清理路径并验证 SQLite 连接池和生产环境密钥。
	cfg.HTTP.StaticDir = filepath.Clean(cfg.HTTP.StaticDir)
	cfg.Database.Path = filepath.Clean(cfg.Database.Path)
	cfg.Storage.WorkNavigationPath = filepath.Clean(cfg.Storage.WorkNavigationPath)
	cfg.Storage.BackupDir = filepath.Clean(cfg.Storage.BackupDir)
	cfg.Storage.PositionUploadDir = filepath.Clean(cfg.Storage.PositionUploadDir)
	cfg.Storage.PositionTempDir = filepath.Clean(cfg.Storage.PositionTempDir)
	if cfg.MigrationsDir != "" {
		cfg.MigrationsDir = filepath.Clean(cfg.MigrationsDir)
	}
	cfg.Migration.MySQL.Host = strings.TrimSpace(cfg.Migration.MySQL.Host)
	cfg.Migration.MySQL.Database = strings.TrimSpace(cfg.Migration.MySQL.Database)
	cfg.Migration.MySQL.User = strings.TrimSpace(cfg.Migration.MySQL.User)
	cfg.Development.UpstreamURL = strings.TrimRight(strings.TrimSpace(cfg.Development.UpstreamURL), "/")
	if cfg.Database.Path == "." || strings.TrimSpace(cfg.Database.Path) == "" {
		return Config{}, fmt.Errorf("SQLite 数据库路径不能为空")
	}
	if cfg.Database.MaxIdleConns > cfg.Database.MaxOpenConns {
		return Config{}, fmt.Errorf("SQLite 空闲连接数不能大于最大连接数")
	}
	if cfg.Database.BusyTimeout <= 0 {
		return Config{}, fmt.Errorf("SQLite 锁等待时间必须大于零")
	}
	if cfg.Environment == "production" && (cfg.Auth.JWTSecret == "" || cfg.Auth.EncryptionKey == "") {
		return Config{}, fmt.Errorf("生产环境必须配置 JWT 与加密密钥")
	}
	if cfg.Environment == "production" && cfg.Development.UpstreamURL != "" {
		return Config{}, fmt.Errorf("生产环境不能配置开发 API 上游")
	}

	return cfg, nil
}

// loadString 使用非空环境变量覆盖字符串配置。
// 输入：lookup 是环境查询函数，key 是变量名，target 是目标字段。
// 输出：无。
// 副作用：可能修改 target。
func loadString(lookup LookupEnv, key string, target *string) {
	// 1. 清理并应用非空环境变量。
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

// loadBool 解析可选布尔环境变量。
// 输入：lookup 是环境查询函数，key 是变量名，target 是目标字段。
// 输出：格式无效时返回带变量名的错误。
// 副作用：可能修改 target。
func loadBool(lookup LookupEnv, key string, target *bool) error {
	// 1. 缺失或空值保持默认配置。
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}

	// 2. 使用标准库接受 true/false、1/0 等常见形式。
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("环境变量 %s 必须是布尔值: %w", key, err)
	}
	*target = parsed
	return nil
}

// loadInt 解析并约束可选整数环境变量。
// 输入：lookup 是环境查询函数，key 是变量名，target 是目标字段，min 和 max 是闭区间。
// 输出：格式或范围无效时返回错误。
// 副作用：可能修改 target。
func loadInt(lookup LookupEnv, key string, target *int, min, max int) error {
	// 1. 缺失或空值保持默认配置。
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}

	// 2. 解析十进制整数并检查范围。
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < min || parsed > max {
		return fmt.Errorf("环境变量 %s 必须在 %d 到 %d 之间", key, min, max)
	}
	*target = parsed
	return nil
}
