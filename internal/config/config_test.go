package config

import (
	"path/filepath"
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

	// 3. 断言令牌有效期和默认数据路径。
	if cfg.Auth.TokenLifetime != 72*time.Hour {
		t.Errorf("TokenLifetime = %s, want %s", cfg.Auth.TokenLifetime, 72*time.Hour)
	}
	if cfg.Database.Path != filepath.Clean("data/aowugong.db") {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, filepath.Clean("data/aowugong.db"))
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

// TestLoadNormalizesPaths 验证数据库与静态目录路径会被清理。
func TestLoadNormalizesPaths(t *testing.T) {
	// 1. 提供包含相对路径片段的配置。
	cfg, err := Load(newLookup(map[string]string{
		"AOWUGONG_DATABASE_PATH": "var/../runtime/aowugong.db",
		"AOWUGONG_STATIC_DIR":    "public/../web/dist",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 2. 断言两个路径均已规范化。
	if cfg.Database.Path != filepath.Clean("var/../runtime/aowugong.db") {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, filepath.Clean("var/../runtime/aowugong.db"))
	}
	if cfg.HTTP.StaticDir != filepath.Clean("public/../web/dist") {
		t.Errorf("StaticDir = %q, want %q", cfg.HTTP.StaticDir, filepath.Clean("public/../web/dist"))
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
