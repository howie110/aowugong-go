package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	_ "modernc.org/sqlite"
)

// OpenSQLite 打开应用唯一使用的 SQLite 文件并应用连接级安全参数。
// 输入：ctx 控制首次连通性检查，cfg 提供文件路径、锁等待和连接池上限。
// 输出：返回可并发读取和串行写入的数据库句柄；配置或文件初始化失败时返回错误。
// 副作用：创建数据库目录和文件，启用 WAL、外键、busy_timeout 与 NORMAL 同步。
func OpenSQLite(ctx context.Context, cfg config.Database) (*sql.DB, error) {
	// 1. 校验配置、解析绝对路径并创建仅当前用户可写的数据目录。
	path, err := resolveSQLitePath(cfg)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建 SQLite 数据目录: %w", err)
	}

	// 2. 把每条新连接都必须执行的 PRAGMA 写入 DSN。
	dsn := sqliteDSN(path, cfg.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(0)

	// 3. 主动建立连接并核对关键运行参数，失败时关闭句柄。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 SQLite %s: %w", path, err)
	}
	if err := verifySQLitePragmas(ctx, db, cfg.BusyTimeout); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("限制 SQLite 文件权限: %w", err)
	}
	return db, nil
}

// ResolveSQLitePath 返回配置指向的 SQLite 绝对文件路径。
// 输入：cfg 提供数据库路径。
// 输出：返回清理后的绝对路径；路径为空或无法解析时返回错误。
// 副作用：无，不创建目录或文件。
func ResolveSQLitePath(cfg config.Database) (string, error) {
	// 1. 复用唯一的路径校验逻辑供备份、页面和启动装配使用。
	return resolveSQLitePath(cfg)
}

// resolveSQLitePath 校验并绝对化 SQLite 文件路径。
// 输入：cfg 提供路径和连接池参数。
// 输出：返回绝对路径；配置无效时返回错误。
// 副作用：无。
func resolveSQLitePath(cfg config.Database) (string, error) {
	// 1. 拒绝目录占位、无效连接池和无锁等待配置。
	if cfg.Path == "" || filepath.Clean(cfg.Path) == "." {
		return "", fmt.Errorf("SQLite 数据库路径不能为空")
	}
	if cfg.MaxOpenConns < 1 || cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > cfg.MaxOpenConns {
		return "", fmt.Errorf("SQLite 连接池参数无效")
	}
	if cfg.BusyTimeout <= 0 {
		return "", fmt.Errorf("SQLite 锁等待时间必须大于零")
	}

	// 2. 使用绝对路径避免服务工作目录变化后打开错误数据库。
	path, err := filepath.Abs(filepath.Clean(cfg.Path))
	if err != nil {
		return "", fmt.Errorf("解析 SQLite 数据库路径: %w", err)
	}
	return path, nil
}

// sqliteDSN 构造对每条连接生效的 SQLite 文件 DSN。
// 输入：path 是绝对文件路径，busyTimeoutMS 是锁等待毫秒数。
// 输出：返回包含 WAL、外键和同步参数的 DSN。
// 副作用：无。
func sqliteDSN(path string, busyTimeoutMS int64) string {
	// 1. 使用 file URL 正确转义 Windows 和 Linux 路径。
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	dsn := url.URL{Scheme: "file", Path: urlPath}
	query := dsn.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "busy_timeout("+strconv.FormatInt(busyTimeoutMS, 10)+")")
	query.Add("_pragma", "synchronous(NORMAL)")
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

// verifySQLitePragmas 确认当前连接应用了生产所需的 SQLite 参数。
// 输入：ctx 控制查询，db 是已打开句柄，busyTimeout 是期望锁等待。
// 输出：参数全部匹配返回 nil，否则返回带实际值的错误。
// 副作用：只读 SQLite PRAGMA。
func verifySQLitePragmas(ctx context.Context, db *sql.DB, busyTimeout time.Duration) error {
	// 1. 分别读取外键、日志、锁等待和同步等级。
	var foreignKeys, timeoutMS, synchronous int
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("读取 SQLite foreign_keys: %w", err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("读取 SQLite journal_mode: %w", err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeoutMS); err != nil {
		return fmt.Errorf("读取 SQLite busy_timeout: %w", err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("读取 SQLite synchronous: %w", err)
	}

	// 2. 使用 SQLite 常量语义校验最终配置。
	if foreignKeys != 1 || journalMode != "wal" || timeoutMS != int(busyTimeout/time.Millisecond) || synchronous != 1 {
		return fmt.Errorf(
			"SQLite 参数无效: foreign_keys=%d journal_mode=%s busy_timeout=%d synchronous=%d",
			foreignKeys, journalMode, timeoutMS, synchronous,
		)
	}
	return nil
}
