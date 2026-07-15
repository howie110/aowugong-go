// Package config 负责加载运行时配置。
package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultEnvironment   = "development"
	defaultHTTPAddress   = "0.0.0.0:2346"
	defaultDatabasePath  = "storage/data/aowugong.db"
	defaultStaticDir     = "web/dist"
	defaultTokenLifetime = 72 * time.Hour
)

// LookupEnv 定义查询环境变量的函数类型。
type LookupEnv func(string) (string, bool)

// Config 汇总应用运行所需的配置。
type Config struct {
	Environment   string
	MigrationsDir string
	HTTP          HTTP
	Database      Database
	Auth          Auth
}

// HTTP 描述 HTTP 服务配置。
type HTTP struct {
	Address   string
	StaticDir string
}

// Database 描述 SQLite 数据库配置。
type Database struct {
	Path string
}

// Auth 描述令牌与加密配置。
type Auth struct {
	JWTSecret     string
	EncryptionKey string
	TokenLifetime time.Duration
}

// Load 从环境变量加载并校验应用配置。
func Load(lookup LookupEnv) (Config, error) {
	// 1. 建立所有运行环境共用的默认配置。
	cfg := Config{
		Environment: defaultEnvironment,
		HTTP: HTTP{
			Address:   defaultHTTPAddress,
			StaticDir: defaultStaticDir,
		},
		Database: Database{Path: defaultDatabasePath},
		Auth: Auth{
			TokenLifetime: defaultTokenLifetime,
		},
	}

	// 2. 使用非空环境变量覆盖默认配置。
	if value, ok := lookup("AOWUGONG_ENV"); ok && strings.TrimSpace(value) != "" {
		cfg.Environment = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_HTTP_ADDRESS"); ok && strings.TrimSpace(value) != "" {
		cfg.HTTP.Address = strings.TrimSpace(value)
	}
	if value, ok := lookup("AOWUGONG_DATABASE_PATH"); ok && strings.TrimSpace(value) != "" {
		cfg.Database.Path = strings.TrimSpace(value)
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

	// 3. 清理路径并验证生产环境的必填密钥。
	cfg.Database.Path = filepath.Clean(cfg.Database.Path)
	cfg.HTTP.StaticDir = filepath.Clean(cfg.HTTP.StaticDir)
	if cfg.MigrationsDir != "" {
		cfg.MigrationsDir = filepath.Clean(cfg.MigrationsDir)
	}
	if cfg.Environment == "production" && (cfg.Auth.JWTSecret == "" || cfg.Auth.EncryptionKey == "") {
		return Config{}, fmt.Errorf("生产环境必须配置 JWT 与加密密钥")
	}

	return cfg, nil
}
