package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	_ "modernc.org/sqlite"
)

var migrationTables = []string{
	"job_execution", "job_execution_lock", "notification_log",
	"aowugong_fastapi_users", "aowugong_roles", "aowugong_permissions",
	"aowugong_user_roles", "aowugong_role_permissions",
	"basic_operation", "basic_position",
	"finance_broker_account", "finance_asset_snapshot", "finance_position_holding_snapshot",
	"investment_article_source", "investment_article", "investment_article_analysis",
	"investment_signal_group", "investment_signal_alias",
	"mahjong_game_record", "service_monitor_result", "subscription_record",
	"tushare_daily", "tushare_etf_basic", "tushare_stock_basic", "tushare_trade_cal",
}

type tableReport struct {
	Name        string `json:"name"`
	SourceCount int64  `json:"source_count"`
	TargetCount int64  `json:"target_count"`
	MinimumID   *int64 `json:"minimum_id,omitempty"`
	MaximumID   *int64 `json:"maximum_id,omitempty"`
}

type migrationReport struct {
	StartedAt  string        `json:"started_at"`
	FinishedAt string        `json:"finished_at"`
	SourcePath string        `json:"source_path"`
	Tables     []tableReport `json:"tables"`
	TotalRows  int64         `json:"total_rows"`
}

// main 校验显式确认后执行 SQLite 到 PostgreSQL 的一次性迁移。
// 输入：--confirm、AOWUGONG_SQLITE_SOURCE_PATH 和 AOWUGONG_DATABASE_URL。
// 输出：标准输出写入逐表核对 JSON；失败时标准错误输出原因并返回非零状态。
// 副作用：只读 SQLite，迁移并重写目标 PostgreSQL 全部业务表。
func main() {
	// 1. 解析参数并使用信号可取消上下文执行完整迁移。
	confirm := flag.Bool("confirm", false, "确认清空并重建 PostgreSQL 业务表数据")
	flag.Parse()
	if err := run(context.Background(), *confirm); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 打开来源和目标、建立结构、复制数据并输出核对报告。
// 输入：ctx 控制迁移，confirmed 必须明确为 true。
// 输出：全部表复制并核对成功返回 nil。
// 副作用：读取 SQLite，执行 PostgreSQL DDL，并在单事务中重写业务数据。
func run(ctx context.Context, confirmed bool) error {
	// 1. 拒绝未确认执行并加载生产连接配置。
	if !confirmed {
		return fmt.Errorf("拒绝迁移：必须显式传入 --confirm")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("加载迁移配置: %w", err)
	}
	sourcePath, err := filepath.Abs(filepath.Clean(cfg.SQLiteSourcePath))
	if err != nil {
		return fmt.Errorf("解析 SQLite 来源路径: %w", err)
	}
	if info, err := os.Stat(sourcePath); err != nil || info.IsDir() {
		return fmt.Errorf("SQLite 来源文件无效: %s", sourcePath)
	}

	// 2. 只读打开 SQLite，打开 PostgreSQL 并应用正式迁移。
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourcePath)+"?mode=ro")
	if err != nil {
		return fmt.Errorf("打开 SQLite 来源: %w", err)
	}
	defer source.Close()
	if err := source.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 SQLite 来源: %w", err)
	}
	var integrity string
	if err := source.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("SQLite 来源完整性异常: result=%s error=%v", integrity, err)
	}
	target, err := database.OpenPostgres(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("打开 PostgreSQL 目标: %w", err)
	}
	defer target.Close()
	migrationsDirectory, err := resolveMigrationsDirectory(cfg.MigrationsDir)
	if err != nil {
		return err
	}
	if err := database.MigratePostgres(ctx, target, migrationsDirectory); err != nil {
		return fmt.Errorf("建立 PostgreSQL 结构: %w", err)
	}

	// 3. 在单个目标事务内清空、复制、重置序列并核对所有表。
	startedAt := time.Now()
	report, err := migrateData(ctx, source, target)
	if err != nil {
		return err
	}
	report.StartedAt = startedAt.Format(time.RFC3339)
	report.FinishedAt = time.Now().Format(time.RFC3339)
	report.SourcePath = sourcePath
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("输出迁移报告: %w", err)
	}
	return nil
}

// migrateData 原子复制全部业务表并核对行数及主键范围。
// 输入：ctx 控制迁移，source 是只读 SQLite，target 是已迁移 PostgreSQL。
// 输出：返回逐表核对报告。
// 副作用：清空并重写 PostgreSQL 业务表，失败时回滚全部数据写入。
func migrateData(ctx context.Context, source, target *sql.DB) (migrationReport, error) {
	// 1. 开启目标事务并一次清空全部正式业务表。
	transaction, err := target.BeginTx(ctx, nil)
	if err != nil {
		return migrationReport{}, fmt.Errorf("开始 PostgreSQL 数据迁移事务: %w", err)
	}
	defer transaction.Rollback()
	quotedTables := make([]string, len(migrationTables))
	for index, table := range migrationTables {
		quotedTables[index] = quoteIdentifier(table)
	}
	if _, err := transaction.ExecContext(ctx, "TRUNCATE TABLE "+strings.Join(quotedTables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return migrationReport{}, fmt.Errorf("清空 PostgreSQL 业务表: %w", err)
	}

	// 2. 按外键依赖顺序逐表复制全部字段。
	report := migrationReport{Tables: make([]tableReport, 0, len(migrationTables))}
	for _, table := range migrationTables {
		item, err := copyTable(ctx, source, transaction, table)
		if err != nil {
			return migrationReport{}, err
		}
		report.Tables = append(report.Tables, item)
		report.TotalRows += item.TargetCount
	}

	// 3. 全部核对成功后一次提交，避免页面看到半迁移状态。
	if err := transaction.Commit(); err != nil {
		return migrationReport{}, fmt.Errorf("提交 PostgreSQL 数据迁移: %w", err)
	}
	return report, nil
}

// copyTable 复制并核对一张结构一致的业务表。
// 输入：ctx 控制复制，source 是 SQLite，target 是 PostgreSQL 事务，table 是白名单表名。
// 输出：返回来源/目标行数和可选主键范围。
// 副作用：读取 SQLite 并向 PostgreSQL 插入当前表全部行。
func copyTable(ctx context.Context, source *sql.DB, target *sql.Tx, table string) (tableReport, error) {
	// 1. 从 SQLite 正式结构读取字段顺序并准备参数化 INSERT。
	columns, hasID, err := sqliteColumns(ctx, source, table)
	if err != nil {
		return tableReport{}, err
	}
	quotedColumns := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for index, column := range columns {
		quotedColumns[index] = quoteIdentifier(column)
		placeholders[index] = "?"
	}
	selectSQL := "SELECT " + strings.Join(quotedColumns, ",") + " FROM " + quoteIdentifier(table)
	insertSQL := "INSERT INTO " + quoteIdentifier(table) + " (" + strings.Join(quotedColumns, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	rows, err := source.QueryContext(ctx, selectSQL)
	if err != nil {
		return tableReport{}, fmt.Errorf("读取 SQLite 表 %s: %w", table, err)
	}
	defer rows.Close()
	statement, err := target.PrepareContext(ctx, insertSQL)
	if err != nil {
		return tableReport{}, fmt.Errorf("准备 PostgreSQL 表 %s 写入: %w", table, err)
	}
	defer statement.Close()

	// 2. 逐行扫描并把 SQLite 文本字节规范为 PostgreSQL 文本参数。
	var sourceCount int64
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return tableReport{}, fmt.Errorf("扫描 SQLite 表 %s: %w", table, err)
		}
		for index, value := range values {
			if bytesValue, ok := value.([]byte); ok {
				values[index] = string(bytesValue)
			}
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return tableReport{}, fmt.Errorf("写入 PostgreSQL 表 %s 第 %d 行: %w", table, sourceCount+1, err)
		}
		sourceCount++
	}
	if err := rows.Err(); err != nil {
		return tableReport{}, fmt.Errorf("遍历 SQLite 表 %s: %w", table, err)
	}

	// 3. 重置自增序列并核对目标行数与主键范围。
	item := tableReport{Name: table, SourceCount: sourceCount}
	if hasID {
		sequenceSQL := "SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM " + quoteIdentifier(table)
		var sequenceValue int64
		if err := target.QueryRowContext(ctx, sequenceSQL, table).Scan(&sequenceValue); err != nil {
			return tableReport{}, fmt.Errorf("重置 PostgreSQL 表 %s 序列: %w", table, err)
		}
		var minimum, maximum sql.NullInt64
		if err := target.QueryRowContext(ctx, "SELECT MIN(id), MAX(id) FROM "+quoteIdentifier(table)).Scan(&minimum, &maximum); err != nil {
			return tableReport{}, fmt.Errorf("核对 PostgreSQL 表 %s 主键范围: %w", table, err)
		}
		if minimum.Valid {
			item.MinimumID, item.MaximumID = &minimum.Int64, &maximum.Int64
		}
	}
	if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table)).Scan(&item.TargetCount); err != nil {
		return tableReport{}, fmt.Errorf("统计 PostgreSQL 表 %s: %w", table, err)
	}
	if item.SourceCount != item.TargetCount {
		return tableReport{}, fmt.Errorf("表 %s 行数不一致: SQLite=%d PostgreSQL=%d", table, item.SourceCount, item.TargetCount)
	}
	return item, nil
}

// sqliteColumns 读取迁移来源表的正式字段顺序。
// 输入：ctx 控制查询，source 是 SQLite，table 是白名单表名。
// 输出：返回字段名和是否存在 id 字段。
// 副作用：只读 SQLite schema。
func sqliteColumns(ctx context.Context, source *sql.DB, table string) ([]string, bool, error) {
	// 1. 使用 PRAGMA table_info 获取稳定字段顺序。
	rows, err := source.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return nil, false, fmt.Errorf("读取 SQLite 表 %s 字段: %w", table, err)
	}
	defer rows.Close()
	columns := make([]string, 0)
	hasID := false
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, false, fmt.Errorf("扫描 SQLite 表 %s 字段: %w", table, err)
		}
		columns = append(columns, name)
		hasID = hasID || name == "id"
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("遍历 SQLite 表 %s 字段: %w", table, err)
	}
	if len(columns) == 0 {
		return nil, false, fmt.Errorf("SQLite 来源缺少表或字段: %s", table)
	}
	return columns, hasID, nil
}

// resolveMigrationsDirectory 定位发布包或源码中的 PostgreSQL 迁移目录。
// 输入：configured 是可选显式目录。
// 输出：返回存在的迁移目录。
// 副作用：只读取路径元数据。
func resolveMigrationsDirectory(configured string) (string, error) {
	// 1. 优先使用显式配置和可执行文件同级发布目录。
	if strings.TrimSpace(configured) != "" {
		return filepath.Clean(configured), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取迁移程序路径: %w", err)
	}
	releaseDirectory := filepath.Join(filepath.Dir(executable), "migrations", "postgres")
	if info, err := os.Stat(releaseDirectory); err == nil && info.IsDir() {
		return releaseDirectory, nil
	}

	// 2. 源码执行时回退到仓库正式迁移目录。
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位 PostgreSQL 迁移目录")
	}
	sourceDirectory := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations", "postgres"))
	if info, err := os.Stat(sourceDirectory); err != nil || !info.IsDir() {
		return "", fmt.Errorf("PostgreSQL 迁移目录不存在: %s", sourceDirectory)
	}
	return sourceDirectory, nil
}

// quoteIdentifier 安全引用固定白名单中的 SQL 标识符。
// 输入：value 是代码维护的表名或字段名。
// 输出：返回双引号包裹的标识符。
// 副作用：无。
func quoteIdentifier(value string) string {
	// 1. 双写内部双引号后包裹标识符。
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
