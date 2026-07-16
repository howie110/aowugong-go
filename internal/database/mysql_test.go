package database

import (
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestNewMySQLDriverConfigBuildsSafeRuntimeOptions 验证 MySQL 驱动参数保留更新语义并设置网络超时。
// 输入：完整的应用数据库配置。
// 输出：驱动配置包含 TCP 地址、库名、上海时区和连接超时。
// 副作用：无，不建立真实数据库连接。
func TestNewMySQLDriverConfigBuildsSafeRuntimeOptions(t *testing.T) {
	// 1. 构造本地 SSH 隧道对应的数据库配置。
	cfg := config.Database{
		Host: "127.0.0.1", Port: 13306, Name: "aowugong", User: "worker", Password: "secret",
		MaxOpenConns: 8, MaxIdleConns: 2, ConnMaxLifetime: 5 * time.Minute,
	}

	// 2. 转换并核对不会改变仓储语义的驱动参数。
	driverCfg, err := newMySQLDriverConfig(cfg)
	if err != nil {
		t.Fatalf("newMySQLDriverConfig() error = %v", err)
	}
	if driverCfg.Net != "tcp" || driverCfg.Addr != "127.0.0.1:13306" || driverCfg.DBName != "aowugong" {
		t.Errorf("driver address = %s/%s/%s", driverCfg.Net, driverCfg.Addr, driverCfg.DBName)
	}
	if !driverCfg.ClientFoundRows {
		t.Error("ClientFoundRows = false, want true")
	}
	if driverCfg.Timeout != 5*time.Second || driverCfg.ReadTimeout != 30*time.Second || driverCfg.WriteTimeout != 30*time.Second {
		t.Errorf("driver timeouts = %s/%s/%s", driverCfg.Timeout, driverCfg.ReadTimeout, driverCfg.WriteTimeout)
	}
	if driverCfg.Loc == nil || driverCfg.Loc.String() != "Asia/Shanghai" {
		t.Errorf("driver location = %v, want Asia/Shanghai", driverCfg.Loc)
	}
}

// TestNewMySQLDriverConfigRejectsIncompleteIdentity 验证数据库身份缺失时不会尝试联网。
// 输入：缺少密码的数据库配置。
// 输出：返回明确校验错误。
// 副作用：无，不建立真实数据库连接。
func TestNewMySQLDriverConfigRejectsIncompleteIdentity(t *testing.T) {
	// 1. 构造缺少密码的配置并执行转换。
	_, err := newMySQLDriverConfig(config.Database{Host: "127.0.0.1", Port: 3306, Name: "aowugong", User: "app"})

	// 2. 断言在驱动初始化前被拒绝。
	if err == nil {
		t.Fatal("newMySQLDriverConfig() error = nil, want validation error")
	}
}
