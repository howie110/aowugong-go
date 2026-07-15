// Package mysqlmigration 提供只读 MySQL 到 SQLite 的一次性数据迁移与核验。
package mysqlmigration

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// SourceConfig 描述已解析的 MySQL DSN 和源库名称。
type SourceConfig struct {
	DSN    string `json:"-"`
	Schema string `json:"schema"`
}

// ParseSourceURL 把 FastAPI SQLAlchemy MySQL URL 转换为 Go MySQL DSN。
// 输入：value 是 mysql:// 或 mysql+pymysql:// URL。
// 输出：返回启用时间解析和上海时区的 DSN 及库名；格式无效时返回错误。
// 副作用：无，不连接数据库。
func ParseSourceURL(value string) (SourceConfig, error) {
	// 1. 解析并限制为 MySQL 协议，拒绝缺少主机、用户或数据库名。
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return SourceConfig{}, fmt.Errorf("解析 MySQL URL: %w", err)
	}
	if parsed.Scheme != "mysql" && !strings.HasPrefix(parsed.Scheme, "mysql+") {
		return SourceConfig{}, fmt.Errorf("源数据库 URL 必须使用 MySQL 协议")
	}
	if parsed.User == nil || parsed.User.Username() == "" || parsed.Hostname() == "" {
		return SourceConfig{}, fmt.Errorf("MySQL URL 缺少用户或主机")
	}
	schema := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if schema == "" || strings.Contains(schema, "/") {
		return SourceConfig{}, fmt.Errorf("MySQL URL 必须包含单个数据库名")
	}
	port := parsed.Port()
	if port == "" {
		port = "3306"
	}
	password, _ := parsed.User.Password()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return SourceConfig{}, fmt.Errorf("加载 MySQL 时区: %w", err)
	}

	// 2. 构造驱动配置并保留安全的字符集参数。
	driverConfig := mysql.NewConfig()
	driverConfig.User = parsed.User.Username()
	driverConfig.Passwd = password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(parsed.Hostname(), port)
	driverConfig.DBName = schema
	driverConfig.ParseTime = true
	driverConfig.Loc = location
	driverConfig.Timeout = 15 * time.Second
	driverConfig.ReadTimeout = 5 * time.Minute
	driverConfig.WriteTimeout = 15 * time.Second
	driverConfig.Params = map[string]string{"charset": "utf8mb4"}
	if charset := strings.TrimSpace(parsed.Query().Get("charset")); charset != "" {
		driverConfig.Params["charset"] = charset
	}
	return SourceConfig{DSN: driverConfig.FormatDSN(), Schema: schema}, nil
}

// ParseSourceDSN 验证原生 Go MySQL DSN 并提取数据库名。
// 输入：value 是 go-sql-driver/mysql DSN。
// 输出：返回强制启用时间解析和上海时区的 DSN；格式无效时返回错误。
// 副作用：无，不连接数据库。
func ParseSourceDSN(value string) (SourceConfig, error) {
	// 1. 使用官方驱动解析器校验并提取数据库名。
	parsed, err := mysql.ParseDSN(strings.TrimSpace(value))
	if err != nil {
		return SourceConfig{}, fmt.Errorf("解析 MySQL DSN: %w", err)
	}
	if strings.TrimSpace(parsed.DBName) == "" {
		return SourceConfig{}, fmt.Errorf("MySQL DSN 必须包含数据库名")
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return SourceConfig{}, fmt.Errorf("加载 MySQL 时区: %w", err)
	}
	parsed.ParseTime = true
	parsed.Loc = location
	if parsed.Timeout == 0 {
		parsed.Timeout = 15 * time.Second
	}
	if parsed.ReadTimeout == 0 {
		parsed.ReadTimeout = 5 * time.Minute
	}
	return SourceConfig{DSN: parsed.FormatDSN(), Schema: parsed.DBName}, nil
}
