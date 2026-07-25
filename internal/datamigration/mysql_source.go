package datamigration

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/howiedata/aowugong-go/internal/config"
)

// OpenMySQLSource 打开一次性迁移工具使用的旧 MySQL 数据库。
// 输入：ctx 控制首次连通性检查，cfg 提供网络地址和数据库身份。
// 输出：返回可并发使用的数据库句柄；配置或连接失败时返回带业务上下文的错误。
// 副作用：建立旧 MySQL 网络连接，不创建表或修改业务数据。
func OpenMySQLSource(ctx context.Context, cfg config.MySQLSource) (*sql.DB, error) {
	// 1. 校验应用配置并构造不会被日志输出的驱动参数。
	driverCfg, err := newMySQLDriverConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 2. 建立只供迁移读取使用的小型连接池。
	db, err := sql.Open("mysql", driverCfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 3. 主动验证网络、账号和目标数据库均可访问。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 MySQL %s: %w", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), err)
	}
	return db, nil
}

// newMySQLDriverConfig 将应用数据库配置转换为 MySQL 驱动参数。
// 输入：cfg 包含网络地址、数据库身份和密码。
// 输出：返回启用匹配行数语义、上海时区和网络超时的驱动配置。
// 副作用：无，不建立数据库连接。
func newMySQLDriverConfig(cfg config.MySQLSource) (*mysql.Config, error) {
	// 1. 在生成 DSN 前拒绝缺少身份或连接池参数的配置。
	host := strings.TrimSpace(cfg.Host)
	databaseName := strings.TrimSpace(cfg.Database)
	user := strings.TrimSpace(cfg.User)
	if host == "" || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("MySQL 主机或端口无效")
	}
	if databaseName == "" || user == "" || cfg.Password == "" {
		return nil, fmt.Errorf("MySQL 数据库名、账号和密码不能为空")
	}
	// 2. 使用固定上海时区和超时，避免迁移工具长期占住线上连接。
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, fmt.Errorf("加载 MySQL 时区 Asia/Shanghai: %w", err)
	}
	return &mysql.Config{
		User: user, Passwd: cfg.Password, Net: "tcp",
		Addr: net.JoinHostPort(host, strconv.Itoa(cfg.Port)), DBName: databaseName,
		Collation: "utf8mb4_0900_ai_ci", Loc: location, ParseTime: false,
		ClientFoundRows: true, AllowNativePasswords: true,
		Timeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
	}, nil
}
