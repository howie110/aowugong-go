package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadUsesDevelopmentDefaults 验证开发环境的默认运行配置。
func TestLoadUsesDevelopmentDefaults(t *testing.T) {
	// 1. 使用空环境加载默认配置。
	cfg, err := Load(newLookup(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言开发环境与监听地址。
	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTP.Address != "0.0.0.0:2346" {
		t.Errorf("HTTP.Address = %q, want 0.0.0.0:2346", cfg.HTTP.Address)
	}

	// 3. 断言令牌有效期和默认 MySQL 连接参数。
	if cfg.Auth.TokenLifetime != 72*time.Hour {
		t.Errorf("TokenLifetime = %s, want %s", cfg.Auth.TokenLifetime, 72*time.Hour)
	}
	if cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 3306 {
		t.Errorf("Database address = %s:%d, want 127.0.0.1:3306", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Database.Name != "aowugong" || cfg.Database.User != "aowugong" {
		t.Errorf("Database identity = %s/%s, want aowugong/aowugong", cfg.Database.Name, cfg.Database.User)
	}
	if cfg.Database.MaxOpenConns != 8 || cfg.Database.MaxIdleConns != 2 {
		t.Errorf("Database pool = %d/%d, want 8/2", cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	}
}

// TestEnvironmentExampleUsesMySQLSettings 验证环境示例只提供 MySQL 运行配置。
func TestEnvironmentExampleUsesMySQLSettings(t *testing.T) {
	// 1. 读取仓库中的环境变量示例。
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", ".env.example"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	// 2. 断言示例包含 MySQL 字段且不再暴露 SQLite 运行路径。
	for _, key := range []string{"AOWUGONG_MYSQL_HOST=", "AOWUGONG_MYSQL_PORT=", "AOWUGONG_MYSQL_DATABASE=", "AOWUGONG_MYSQL_USER=", "AOWUGONG_MYSQL_PASSWORD="} {
		if !strings.Contains(string(content), key) {
			t.Errorf(".env.example missing %s", key)
		}
	}
	if strings.Contains(string(content), "AOWUGONG_DATABASE_PATH=") {
		t.Error(".env.example still contains SQLite database path")
	}
}

// TestLoadUsesMySQLOverrides 验证 MySQL 地址、身份和连接池均可由环境变量覆盖。
func TestLoadUsesMySQLOverrides(t *testing.T) {
	// 1. 提供本地 SSH 隧道和受限任务账号配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_MYSQL_HOST":            "127.0.0.1",
		"AOWUGONG_MYSQL_PORT":            "13306",
		"AOWUGONG_MYSQL_DATABASE":        "aowugong",
		"AOWUGONG_MYSQL_USER":            "aowugong_worker",
		"AOWUGONG_MYSQL_PASSWORD":        "test-password",
		"AOWUGONG_MYSQL_MAX_OPEN_CONNS":  "12",
		"AOWUGONG_MYSQL_MAX_IDLE_CONNS":  "3",
		"AOWUGONG_MYSQL_SKIP_MIGRATIONS": "true",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言全部网络数据库参数已加载。
	if cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 13306 {
		t.Errorf("Database address = %s:%d, want 127.0.0.1:13306", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Database.User != "aowugong_worker" || cfg.Database.Password != "test-password" {
		t.Errorf("Database credentials were not loaded")
	}
	if cfg.Database.MaxOpenConns != 12 || cfg.Database.MaxIdleConns != 3 {
		t.Errorf("Database pool = %d/%d, want 12/3", cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	}
	if !cfg.Database.SkipMigrations {
		t.Error("Database.SkipMigrations = false, want true")
	}
}

// TestLoadUsesHTTPAddressOverride 验证 HTTP 地址可由环境变量覆盖。
func TestLoadUsesHTTPAddressOverride(t *testing.T) {
	// 1. 提供开发环境的自定义监听地址。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_HTTP_ADDRESS": "127.0.0.1:3456",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言环境变量覆盖默认开发地址。
	if cfg.HTTP.Address != "127.0.0.1:3456" {
		t.Errorf("HTTP.Address = %q, want 127.0.0.1:3456", cfg.HTTP.Address)
	}
}

// TestLoadRequiresProductionSecrets 验证生产环境要求两个密钥。
func TestLoadRequiresProductionSecrets(t *testing.T) {
	// 1. 以缺少密钥的生产环境加载配置。
	_, err := Load(newLookup(map[string]string{
		"AOWUGONG_ENV": "production",
	}))

	// 2. 断言返回配置错误。
	if err == nil {
		t.Fatal("Load() error = nil, want production secret validation error")
	}
}

// TestLoadAcceptsProductionSecrets 验证生产环境接受完整密钥配置。
func TestLoadAcceptsProductionSecrets(t *testing.T) {
	// 1. 以完整生产密钥加载配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_ENV":            "production",
		"AOWUGONG_JWT_SECRET":     "jwt-secret",
		"AOWUGONG_ENCRYPTION_KEY": "encryption-key",
		"AOWUGONG_MYSQL_PASSWORD": "mysql-password",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言密钥被保留在认证配置中。
	if cfg.Auth.JWTSecret != "jwt-secret" {
		t.Errorf("JWTSecret = %q, want jwt-secret", cfg.Auth.JWTSecret)
	}
	if cfg.Auth.EncryptionKey != "encryption-key" {
		t.Errorf("EncryptionKey = %q, want encryption-key", cfg.Auth.EncryptionKey)
	}
}

// TestLoadRequiresProductionDatabasePassword 验证生产环境拒绝缺少 MySQL 密码。
func TestLoadRequiresProductionDatabasePassword(t *testing.T) {
	// 1. 提供应用密钥但故意省略数据库密码。
	_, err := Load(newLookup(map[string]string{
		"AOWUGONG_ENV":            "production",
		"AOWUGONG_JWT_SECRET":     "jwt-secret",
		"AOWUGONG_ENCRYPTION_KEY": "encryption-key",
	}))

	// 2. 断言配置加载失败且错误指向 MySQL。
	if err == nil || !strings.Contains(err.Error(), "MySQL") {
		t.Fatalf("Load() error = %v, want MySQL validation error", err)
	}
}

// TestLoadNormalizesPaths 验证静态目录路径会被清理。
func TestLoadNormalizesPaths(t *testing.T) {
	// 1. 提供包含相对路径片段的静态目录配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_STATIC_DIR": "public/../web/dist",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言静态目录已规范化。
	if cfg.HTTP.StaticDir != filepath.Clean("public/../web/dist") {
		t.Errorf("StaticDir = %q, want %q", cfg.HTTP.StaticDir, filepath.Clean("public/../web/dist"))
	}
}

// TestLoadUsesMigrationsDirectoryOverride 验证迁移目录可由环境变量覆盖并规范化。
func TestLoadUsesMigrationsDirectoryOverride(t *testing.T) {
	// 1. 提供包含相对路径片段的迁移目录。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_MIGRATIONS_DIR": "deploy/../release/migrations",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言显式迁移目录已被保留并规范化。
	want := filepath.Clean("deploy/../release/migrations")
	if cfg.MigrationsDir != want {
		t.Errorf("MigrationsDir = %q, want %q", cfg.MigrationsDir, want)
	}
}

// newLookup 创建供配置测试使用的环境查询函数。
func newLookup(values map[string]string) LookupEnv {
	// 1. 返回与操作系统环境兼容的查询函数。
	return func(key string) (string, bool) {
		// 2. 查询测试提供的环境变量。
		value, ok := values[key]
		return value, ok
	}
}
