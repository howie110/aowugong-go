package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgres 打开 PostgreSQL 连接池并验证当前数据库连接。
// 输入：ctx 控制首次连通检查，cfg 提供 DSN、连接池和连接生命周期。
// 输出：返回自动转换问号占位符的 database/sql 连接池；配置或连接失败时返回错误。
// 副作用：连接 PostgreSQL，并为连接设置 Asia/Shanghai 时区和应用名称。
func OpenPostgres(ctx context.Context, cfg config.Database) (*sql.DB, error) {
	// 1. 校验连接参数并解析 pgx 原生连接配置。
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("PostgreSQL 连接地址不能为空")
	}
	if cfg.MaxOpenConns < 1 || cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > cfg.MaxOpenConns {
		return nil, fmt.Errorf("PostgreSQL 连接池参数无效")
	}
	if cfg.ConnMaxLifetime <= 0 {
		return nil, fmt.Errorf("PostgreSQL 连接生命周期必须大于零")
	}
	connectionConfig, err := pgx.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("解析 PostgreSQL 连接地址: %w", err)
	}
	if connectionConfig.RuntimeParams == nil {
		connectionConfig.RuntimeParams = make(map[string]string)
	}
	connectionConfig.RuntimeParams["timezone"] = "Asia/Shanghai"
	connectionConfig.RuntimeParams["application_name"] = "aowugong-go"

	// 2. 包装 pgx connector，让现有仓储继续使用 database/sql 问号占位符。
	connector := &rebindConnector{Connector: stdlib.GetConnector(*connectionConfig)}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 3. 主动连通并确认服务端时区，失败时关闭连接池。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	var timezone string
	if err := db.QueryRowContext(ctx, "SHOW timezone").Scan(&timezone); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("读取 PostgreSQL 时区: %w", err)
	}
	if timezone != "Asia/Shanghai" {
		_ = db.Close()
		return nil, fmt.Errorf("PostgreSQL 时区无效: %s", timezone)
	}
	return db, nil
}

// rebindConnector 为 pgx 连接增加问号占位符转换。
type rebindConnector struct {
	driver.Connector
}

// Connect 建立并包装一条 PostgreSQL 驱动连接。
// 输入：ctx 控制连接建立。
// 输出：返回支持 SQL 转换的驱动连接。
// 副作用：建立一条 PostgreSQL 网络连接。
func (c *rebindConnector) Connect(ctx context.Context) (driver.Conn, error) {
	// 1. 连接底层驱动并保留其全部可选能力。
	connection, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: connection}, nil
}

// rebindConn 在 SQL 进入 pgx 前转换占位符。
type rebindConn struct {
	driver.Conn
}

// Prepare 转换并预编译 SQL。
// 输入：query 使用问号占位符。
// 输出：返回底层 PostgreSQL statement。
// 副作用：在当前连接预编译 SQL。
func (c *rebindConn) Prepare(query string) (driver.Stmt, error) {
	// 1. 统一转换后交给底层连接。
	return c.Conn.Prepare(RebindPostgres(query))
}

// PrepareContext 转换并按上下文预编译 SQL。
// 输入：ctx 控制操作，query 使用问号占位符。
// 输出：返回底层 PostgreSQL statement。
// 副作用：在当前连接预编译 SQL。
func (c *rebindConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	// 1. 优先保留底层驱动的上下文取消能力。
	if connection, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return connection.PrepareContext(ctx, RebindPostgres(query))
	}
	return c.Prepare(query)
}

// ExecContext 转换并执行不返回行的 SQL。
// 输入：ctx 控制操作，query 使用问号占位符，args 是已命名参数。
// 输出：返回执行结果。
// 副作用：按 SQL 内容读写 PostgreSQL。
func (c *rebindConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	// 1. 转换占位符并保留 pgx 的上下文执行能力。
	connection, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return connection.ExecContext(ctx, RebindPostgres(query), args)
}

// QueryContext 转换并执行返回行的 SQL。
// 输入：ctx 控制操作，query 使用问号占位符，args 是已命名参数。
// 输出：返回驱动行游标。
// 副作用：读取 PostgreSQL。
func (c *rebindConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// 1. 转换占位符并保留 pgx 的上下文查询能力。
	connection, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return connection.QueryContext(ctx, RebindPostgres(query), args)
}

// BeginTx 使用底层驱动开始事务。
// 输入：ctx 控制操作，opts 描述隔离级别和只读状态。
// 输出：返回驱动事务。
// 副作用：在 PostgreSQL 开始事务。
func (c *rebindConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	// 1. 保留 pgx 的事务上下文与隔离级别支持。
	connection, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, driver.ErrSkip
	}
	return connection.BeginTx(ctx, opts)
}

// Ping 验证当前驱动连接。
// 输入：ctx 控制探测。
// 输出：连接有效时返回 nil。
// 副作用：向 PostgreSQL 发送轻量探测。
func (c *rebindConn) Ping(ctx context.Context) error {
	// 1. 转发底层连接探测。
	if connection, ok := c.Conn.(driver.Pinger); ok {
		return connection.Ping(ctx)
	}
	return nil
}

// ResetSession 在连接复用前恢复会话。
// 输入：ctx 控制恢复操作。
// 输出：会话可继续使用时返回 nil。
// 副作用：可能重置 PostgreSQL 会话状态。
func (c *rebindConn) ResetSession(ctx context.Context) error {
	// 1. 转发 pgx 会话重置能力。
	if connection, ok := c.Conn.(driver.SessionResetter); ok {
		return connection.ResetSession(ctx)
	}
	return nil
}

// IsValid 判断连接是否仍适合回连接池。
// 输入：无。
// 输出：底层连接有效时返回 true。
// 副作用：无。
func (c *rebindConn) IsValid() bool {
	// 1. 转发底层连接有效性判断。
	if connection, ok := c.Conn.(driver.Validator); ok {
		return connection.IsValid()
	}
	return true
}

// CheckNamedValue 让底层驱动转换 SQL 参数。
// 输入：value 是 database/sql 准备传给驱动的参数。
// 输出：参数可接受时返回 nil，否则返回转换错误。
// 副作用：可能规范参数值。
func (c *rebindConn) CheckNamedValue(value *driver.NamedValue) error {
	// 1. 保留 pgx 对时间和数值参数的转换逻辑。
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return driver.ErrSkip
}

// RebindPostgres 把 SQL 中未被引号包裹的问号转换为 PostgreSQL 位置参数。
// 输入：query 是应用仓储使用的问号占位符 SQL。
// 输出：返回使用 $1、$2 顺序占位符的 SQL。
// 副作用：无。
func RebindPostgres(query string) string {
	// 1. 顺序扫描单引号和双引号，避免改写字符串及标识符内部问号。
	var builder strings.Builder
	builder.Grow(len(query) + 8)
	parameter := 1
	inSingleQuote := false
	inDoubleQuote := false
	for index := 0; index < len(query); index++ {
		character := query[index]
		switch character {
		case '\'':
			builder.WriteByte(character)
			if !inDoubleQuote {
				if inSingleQuote && index+1 < len(query) && query[index+1] == '\'' {
					builder.WriteByte(query[index+1])
					index++
					continue
				}
				inSingleQuote = !inSingleQuote
			}
		case '"':
			builder.WriteByte(character)
			if !inSingleQuote {
				if inDoubleQuote && index+1 < len(query) && query[index+1] == '"' {
					builder.WriteByte(query[index+1])
					index++
					continue
				}
				inDoubleQuote = !inDoubleQuote
			}
		case '?':
			if inSingleQuote || inDoubleQuote {
				builder.WriteByte(character)
				continue
			}
			builder.WriteString(fmt.Sprintf("$%d", parameter))
			parameter++
		default:
			builder.WriteByte(character)
		}
	}
	return builder.String()
}

// TimestampText 把时间规范为数据库统一保存的上海时区文本。
// 输入：value 是任意时区时间。
// 输出：返回 YYYY-MM-DD HH:MM:SS 文本。
// 副作用：无。
func TimestampText(value time.Time) string {
	// 1. 统一转换到固定上海时区后去除亚秒，保持历史文本排序语义。
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return value.In(location).Format("2006-01-02 15:04:05")
}
