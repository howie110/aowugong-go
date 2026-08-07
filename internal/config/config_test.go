package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadUsesDevelopmentDefaults 验证开发环境的默认运行配置。
// 输入：空环境查询函数。
// 输出：返回 2345、72 小时令牌和 SQLite 默认参数。
// 副作用：无。
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
	if cfg.HTTP.Address != "0.0.0.0:2345" {
		t.Errorf("HTTP.Address = %q, want 0.0.0.0:2345", cfg.HTTP.Address)
	}

	// 3. 断言令牌有效期和默认 SQLite 连接参数。
	if cfg.Auth.TokenLifetime != 72*time.Hour {
		t.Errorf("TokenLifetime = %s, want %s", cfg.Auth.TokenLifetime, 72*time.Hour)
	}
	if cfg.Database.Path != filepath.Clean("storage/data/aowugong.db") {
		t.Errorf("Database.Path = %q, want storage/data/aowugong.db", cfg.Database.Path)
	}
	if cfg.Database.MaxOpenConns != 4 || cfg.Database.MaxIdleConns != 2 || cfg.Database.BusyTimeout != 5*time.Second {
		t.Errorf("Database = pool %d/%d timeout %s, want 4/2 and 5s",
			cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.BusyTimeout)
	}
}

// TestEnvironmentExampleUsesSQLiteRuntimeSettings 验证环境示例以 SQLite 作为运行时数据库。
// 输入：仓库 configs/.env.example。
// 输出：示例包含 SQLite、一次性 MySQL 来源和 Miniflux API 字段，不再包含旧 RSS 配置。
// 副作用：读取配置模板文件。
func TestEnvironmentExampleUsesSQLiteRuntimeSettings(t *testing.T) {
	// 1. 读取仓库中的环境变量示例。
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", ".env.example"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	// 2. 断言示例包含 SQLite、一次性 MySQL 迁移来源和 Miniflux API。
	for _, key := range []string{
		"AOWUGONG_SQLITE_PATH=", "AOWUGONG_SQLITE_BUSY_TIMEOUT_MS=",
		"AOWUGONG_MYSQL_HOST=", "AOWUGONG_MYSQL_PORT=", "AOWUGONG_MYSQL_DATABASE=",
		"MINIFLUX_BASE_URL=", "MINIFLUX_MONITOR_URL=", "MINIFLUX_API_TOKEN=", "MINIFLUX_CATEGORY=",
	} {
		if !strings.Contains(string(content), key) {
			t.Errorf(".env.example missing %s", key)
		}
	}

	// 3. 旧 WeChatRSS 配置不能继续出现在唯一模板中。
	for _, key := range []string{"INVESTMENT_ARTICLE_AGGREGATE_RSS_URL=", "WECHAT_RSS_MONITOR_URL="} {
		if strings.Contains(string(content), key) {
			t.Errorf(".env.example still contains retired %s", key)
		}
	}
}

// TestLoadUsesSQLiteAndMigrationOverrides 验证运行库和一次性迁移来源可分别覆盖。
// 输入：完整 SQLite 和 MySQL 环境变量映射。
// 输出：配置分别保存运行路径、连接池和迁移来源。
// 副作用：无。
func TestLoadUsesSQLiteAndMigrationOverrides(t *testing.T) {
	// 1. 提供 SQLite 运行参数和旧 MySQL 来源配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_SQLITE_PATH":            "tmp/test.db",
		"AOWUGONG_SQLITE_MAX_OPEN_CONNS":  "6",
		"AOWUGONG_SQLITE_MAX_IDLE_CONNS":  "1",
		"AOWUGONG_SQLITE_BUSY_TIMEOUT_MS": "8000",
		"AOWUGONG_SQLITE_SKIP_MIGRATIONS": "true",
		"AOWUGONG_MYSQL_HOST":             "127.0.0.1",
		"AOWUGONG_MYSQL_PORT":             "13306",
		"AOWUGONG_MYSQL_DATABASE":         "aowugong",
		"AOWUGONG_MYSQL_USER":             "aowugong_worker",
		"AOWUGONG_MYSQL_PASSWORD":         "test-password",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言运行库与迁移来源没有混用。
	if cfg.Database.Path != filepath.Clean("tmp/test.db") {
		t.Errorf("Database.Path = %q", cfg.Database.Path)
	}
	if cfg.Database.MaxOpenConns != 6 || cfg.Database.MaxIdleConns != 1 || cfg.Database.BusyTimeout != 8*time.Second {
		t.Errorf("Database = %#v", cfg.Database)
	}
	if cfg.Migration.MySQL.Host != "127.0.0.1" || cfg.Migration.MySQL.Port != 13306 ||
		cfg.Migration.MySQL.User != "aowugong_worker" || cfg.Migration.MySQL.Password != "test-password" {
		t.Errorf("Migration.MySQL = %#v", cfg.Migration.MySQL)
	}
	if !cfg.Database.SkipMigrations {
		t.Error("Database.SkipMigrations = false, want true")
	}
}

// TestLoadUsesHTTPAddressOverride 验证 HTTP 地址可由环境变量覆盖。
// 输入：自定义 AOWUGONG_HTTP_ADDRESS。
// 输出：配置使用指定监听地址。
// 副作用：无。
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

// TestLoadUsesMinifluxOverrides 验证投资文章来源通过 Miniflux API 独立配置。
// 输入：Miniflux 根地址、API Token 和投资文章分类。
// 输出：配置保留清理后的连接参数，不依赖旧 WeChatRSS 地址。
// 副作用：无。
func TestLoadUsesMinifluxOverrides(t *testing.T) {
	// 1. 提供完整的 Miniflux API 配置。
	cfg, err := Load(newLookup(map[string]string{
		"MINIFLUX_BASE_URL":    "http://127.0.0.1:5000/",
		"MINIFLUX_MONITOR_URL": "http://8.138.123.59:5000/",
		"MINIFLUX_API_TOKEN":   "test-token",
		"MINIFLUX_CATEGORY":    "投资文章",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言文章客户端参数进入唯一配置结构。
	if cfg.Clients.Miniflux.BaseURL != "http://127.0.0.1:5000/" ||
		cfg.Clients.Miniflux.MonitorURL != "http://8.138.123.59:5000/" ||
		cfg.Clients.Miniflux.APIToken != "test-token" || cfg.Clients.Miniflux.Category != "投资文章" {
		t.Errorf("Clients.Miniflux = %#v", cfg.Clients.Miniflux)
	}
}

// TestLoadRequiresProductionSecrets 验证生产环境要求两个密钥。
// 输入：缺少 JWT 和加密密钥的生产环境。
// 输出：加载返回生产密钥校验错误。
// 副作用：无。
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
// 输入：包含 JWT 与加密密钥的生产环境。
// 输出：配置加载成功并保留密钥。
// 副作用：无。
func TestLoadAcceptsProductionSecrets(t *testing.T) {
	// 1. 以完整生产密钥加载配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_ENV":            "production",
		"AOWUGONG_JWT_SECRET":     "jwt-secret",
		"AOWUGONG_ENCRYPTION_KEY": "encryption-key",
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

// TestLoadRejectsProductionDevelopmentUpstream 验证生产环境不能代理另一套 API。
// 输入：配置开发上游地址的生产环境。
// 输出：加载返回开发代理禁用错误。
// 副作用：无。
func TestLoadRejectsProductionDevelopmentUpstream(t *testing.T) {
	// 1. 提供完整密钥并故意配置开发上游。
	_, err := Load(newLookup(map[string]string{
		"AOWUGONG_ENV":              "production",
		"AOWUGONG_JWT_SECRET":       "jwt-secret",
		"AOWUGONG_ENCRYPTION_KEY":   "encryption-key",
		"AOWUGONG_DEV_UPSTREAM_URL": "http://8.138.123.59:2345",
	}))

	// 2. 断言配置加载失败且错误指向开发上游。
	if err == nil || !strings.Contains(err.Error(), "开发 API 上游") {
		t.Fatalf("Load() error = %v, want development upstream validation error", err)
	}
}

// TestLoadNormalizesPaths 验证静态目录路径会被清理。
// 输入：包含冗余路径段的静态与 SQLite 路径。
// 输出：配置返回清理后的平台路径。
// 副作用：无。
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
// 输入：包含冗余路径段的迁移目录环境变量。
// 输出：配置返回清理后的迁移目录。
// 副作用：无。
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
// 输入：环境键值映射。
// 输出：返回符合 LookupEnv 契约的闭包。
// 副作用：无。
func newLookup(values map[string]string) LookupEnv {
	// 1. 返回与操作系统环境兼容的查询函数。
	return func(key string) (string, bool) {
		// 2. 查询测试提供的环境变量。
		value, ok := values[key]
		return value, ok
	}
}
