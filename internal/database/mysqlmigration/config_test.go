package mysqlmigration

import (
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestParseSourceURLConvertsSQLAlchemyMySQLURL 验证旧项目 DATABASE_URL 可直接转换为 Go DSN。
// 输入：包含驱动后缀、转义用户名密码、端口和 charset 的 SQLAlchemy URL。
// 输出：返回 parseTime、上海时区和数据库名正确的只读源配置。
// 副作用：无。
func TestParseSourceURLConvertsSQLAlchemyMySQLURL(t *testing.T) {
	// 1. 解析旧 FastAPI 常用的 mysql+pymysql URL。
	source, err := ParseSourceURL("mysql+pymysql://user%40name:p%3Ass@db.example:3307/aowugong?charset=utf8mb4")
	if err != nil {
		t.Fatalf("ParseSourceURL() error = %v", err)
	}
	parsed, err := mysql.ParseDSN(source.DSN)
	if err != nil {
		t.Fatalf("mysql.ParseDSN() error = %v", err)
	}

	// 2. 核对凭据解码、网络地址、库名和时间转换配置。
	if parsed.User != "user@name" || parsed.Passwd != "p:ss" || parsed.Addr != "db.example:3307" {
		t.Errorf("parsed credentials/address = user:%q password:%q addr:%q", parsed.User, parsed.Passwd, parsed.Addr)
	}
	if source.Schema != "aowugong" || parsed.DBName != "aowugong" || !parsed.ParseTime || parsed.Loc.String() != "Asia/Shanghai" {
		t.Errorf("source = %+v parsed db=%q parseTime=%t loc=%s", source, parsed.DBName, parsed.ParseTime, parsed.Loc)
	}
}

// TestParseSourceURLRejectsNonMySQLAndMissingDatabase 验证错误源地址在连接前被拒绝。
// 输入：PostgreSQL URL 和缺少数据库名的 MySQL URL。
// 输出：两者均返回参数错误。
// 副作用：无。
func TestParseSourceURLRejectsNonMySQLAndMissingDatabase(t *testing.T) {
	// 1. 逐项确认不受支持或不完整的地址无法生成 DSN。
	for _, value := range []string{"postgres://user:pass@db/app", "mysql://user:pass@db/"} {
		if _, err := ParseSourceURL(value); err == nil {
			t.Errorf("ParseSourceURL(%q) error = nil, want validation error", value)
		}
	}
}
