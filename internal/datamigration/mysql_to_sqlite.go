// Package datamigration 提供一次性数据库转换能力。
package datamigration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// TableResult 描述单表迁移和核对结果。
type TableResult struct {
	Name       string   `json:"name"`
	SourceRows int64    `json:"source_rows"`
	TargetRows int64    `json:"target_rows"`
	Skipped    bool     `json:"skipped"`
	KeyColumns []string `json:"key_columns,omitempty"`
	DateColumn string   `json:"date_column,omitempty"`
	DateMin    string   `json:"date_min,omitempty"`
	DateMax    string   `json:"date_max,omitempty"`
	Samples    int      `json:"samples"`
}

// Report 描述一次 MySQL 到 SQLite 迁移的完整核对结果。
type Report struct {
	Tables               []TableResult `json:"tables"`
	MigratedRows         int64         `json:"migrated_rows"`
	SkippedDailyRows     int64         `json:"skipped_daily_rows"`
	SQLiteIntegrityCheck string        `json:"sqlite_integrity_check"`
}

type tableSpec struct {
	name       string
	keyColumns []string
	dateColumn string
}

var migrationTables = []tableSpec{
	{name: "aowugong_fastapi_users", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "aowugong_roles", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "aowugong_permissions", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "aowugong_user_roles", keyColumns: []string{"user_id", "role_id"}},
	{name: "aowugong_role_permissions", keyColumns: []string{"role_id", "permission_id"}},
	{name: "basic_operation", keyColumns: []string{"id"}, dateColumn: "trade_date"},
	{name: "basic_position", keyColumns: []string{"id"}, dateColumn: "trade_date"},
	{name: "finance_broker_account", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "finance_asset_snapshot", keyColumns: []string{"id"}, dateColumn: "snapshot_date"},
	{name: "finance_position_holding_snapshot", keyColumns: []string{"id"}, dateColumn: "snapshot_date"},
	{name: "investment_article_source", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "investment_article", keyColumns: []string{"id"}, dateColumn: "published_at"},
	{name: "investment_article_analysis", keyColumns: []string{"id"}, dateColumn: "analyzed_at"},
	{name: "investment_signal_group", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "investment_signal_alias", keyColumns: []string{"id"}, dateColumn: "created_at"},
	{name: "mahjong_game_record", keyColumns: []string{"id"}, dateColumn: "played_date"},
	{name: "service_monitor_result", keyColumns: []string{"id"}, dateColumn: "checked_at"},
	{name: "subscription_record", keyColumns: []string{"id"}, dateColumn: "expires_on"},
	{name: "tushare_etf_basic", keyColumns: []string{"ts_code", "index_code", "exchange"}, dateColumn: "update_date"},
	{name: "tushare_stock_basic", keyColumns: []string{"ts_code", "symbol"}, dateColumn: "update_date"},
	{name: "tushare_trade_cal", keyColumns: []string{"exchange", "cal_date"}, dateColumn: "cal_date"},
	{name: "notification_log", keyColumns: []string{"id"}, dateColumn: "sent_at"},
	{name: "job_execution", keyColumns: []string{"id"}, dateColumn: "started_at"},
}

// MySQLToSQLite 把当前有效 MySQL 数据迁移到已完成版本迁移的 SQLite。
// 输入：ctx 控制迁移，source 是只读 MySQL，target 是 SQLite，progress 接收简短进度。
// 输出：返回逐表行数和完整性核对报告；任一步失败时回滚 SQLite。
// 副作用：只读 MySQL，清空并重写 SQLite 业务表；不迁移 tushare_daily 历史行。
func MySQLToSQLite(
	ctx context.Context,
	source *sql.DB,
	target *sql.DB,
	progress io.Writer,
) (Report, error) {
	// 1. 校验连接并开启一致性只读源事务和原子目标事务。
	if source == nil || target == nil {
		return Report{}, fmt.Errorf("MySQL 来源和 SQLite 目标不能为空")
	}
	sourceTx, err := source.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Report{}, fmt.Errorf("开始 MySQL 一致性读取: %w", err)
	}
	defer sourceTx.Rollback()
	targetTx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, fmt.Errorf("开始 SQLite 迁移事务: %w", err)
	}
	defer targetTx.Rollback()

	// 2. 按外键反序清空目标业务表，并始终清空个股日线和临时任务锁。
	if err := clearSQLiteTarget(ctx, targetTx); err != nil {
		return Report{}, err
	}

	// 3. 按外键正序复制每张有效表并立即核对行数。
	report := Report{Tables: make([]TableResult, 0, len(migrationTables)+1)}
	for _, spec := range migrationTables {
		result, err := copyTable(ctx, sourceTx, targetTx, spec)
		if err != nil {
			return report, err
		}
		report.Tables = append(report.Tables, result)
		report.MigratedRows += result.TargetRows
		if progress != nil {
			_, _ = fmt.Fprintf(progress, "%s: %d\n", result.Name, result.TargetRows)
		}
	}

	// 4. 只统计被明确排除的个股日线，不读取或写入其大表内容。
	skippedRows, err := countMySQLRows(ctx, sourceTx, "tushare_daily")
	if err != nil {
		return report, fmt.Errorf("统计跳过的 MySQL tushare_daily: %w", err)
	}
	report.SkippedDailyRows = skippedRows
	report.Tables = append(report.Tables, TableResult{
		Name: "tushare_daily", SourceRows: skippedRows, TargetRows: 0, Skipped: true,
	})

	// 5. 在提交前执行 SQLite 结构和外键检查，确保检查失败仍可完整回滚。
	integrityCheck, err := verifySQLiteTarget(ctx, targetTx)
	if err != nil {
		return report, err
	}
	report.SQLiteIntegrityCheck = integrityCheck
	if err := targetTx.Commit(); err != nil {
		return report, fmt.Errorf("提交 SQLite 迁移事务: %w", err)
	}
	return report, nil
}

// clearSQLiteTarget 清空目标业务数据和旧自增序号。
// 输入：ctx 控制写入，target 是目标 SQLite 迁移事务。
// 输出：全部目标表清理完成返回 nil；任一删除失败返回错误。
// 副作用：删除 SQLite 业务行、任务锁和 sqlite_sequence 记录。
func clearSQLiteTarget(ctx context.Context, target *sql.Tx) error {
	// 1. 先清理不参与 MySQL 复制的任务锁、个股日线和全部业务表。
	if _, err := target.ExecContext(ctx, "DELETE FROM job_execution_lock"); err != nil {
		return fmt.Errorf("清空 SQLite job_execution_lock: %w", err)
	}
	if _, err := target.ExecContext(ctx, "DELETE FROM tushare_daily"); err != nil {
		return fmt.Errorf("清空 SQLite tushare_daily: %w", err)
	}
	for index := len(migrationTables) - 1; index >= 0; index-- {
		table := migrationTables[index].name
		if _, err := target.ExecContext(ctx, "DELETE FROM "+quoteSQLiteIdentifier(table)); err != nil {
			return fmt.Errorf("清空 SQLite %s: %w", table, err)
		}
	}
	if _, err := target.ExecContext(ctx, "DELETE FROM sqlite_sequence"); err != nil {
		return fmt.Errorf("重置 SQLite 自增序号: %w", err)
	}
	return nil
}

// verifySQLiteTarget 在提交迁移事务前检查 SQLite 结构和外键完整性。
// 输入：ctx 控制检查，target 是已经写完全部业务数据的目标事务。
// 输出：完整性正常返回 ok；结构损坏或存在孤立外键时返回错误。
// 副作用：只读目标事务。
func verifySQLiteTarget(ctx context.Context, target *sql.Tx) (string, error) {
	// 1. 要求 SQLite 结构检查明确返回 ok。
	var integrityCheck string
	if err := target.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityCheck); err != nil {
		return "", fmt.Errorf("检查 SQLite 完整性: %w", err)
	}
	if integrityCheck != "ok" {
		return "", fmt.Errorf("SQLite 完整性异常: %s", integrityCheck)
	}

	// 2. 单独执行外键检查，因为 integrity_check 不报告外键约束错误。
	rows, err := target.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", fmt.Errorf("检查 SQLite 外键: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return "", fmt.Errorf("扫描 SQLite 外键异常: %w", err)
		}
		return "", fmt.Errorf(
			"SQLite 外键异常: table=%s rowid=%v parent=%s foreign_key=%d",
			table, rowID, parent, foreignKeyID,
		)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历 SQLite 外键检查: %w", err)
	}
	return integrityCheck, nil
}

// copyTable 把一张 MySQL 表按 SQLite 目标字段顺序复制并核对行数。
// 输入：ctx 控制迁移，source 是源事务，target 是目标事务，table 是白名单表名。
// 输出：返回源目标行数；字段、读取、写入或核对失败时返回错误。
// 副作用：读取 MySQL 并写入 SQLite 指定表。
func copyTable(ctx context.Context, source, target *sql.Tx, spec tableSpec) (TableResult, error) {
	// 1. 从 SQLite 正式结构读取字段，避免人工维护第二份字段模型。
	table := spec.name
	columns, err := sqliteTableColumns(ctx, target, table)
	if err != nil {
		return TableResult{}, err
	}
	if len(columns) == 0 {
		return TableResult{}, fmt.Errorf("SQLite 表 %s 没有字段", table)
	}
	quotedMySQLColumns := make([]string, len(columns))
	quotedSQLiteColumns := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for index, column := range columns {
		quotedMySQLColumns[index] = quoteMySQLIdentifier(column)
		quotedSQLiteColumns[index] = quoteSQLiteIdentifier(column)
		placeholders[index] = "?"
	}

	// 2. 流式读取源表并复用目标预编译插入语句。
	query := "SELECT " + strings.Join(quotedMySQLColumns, ",") + " FROM " + quoteMySQLIdentifier(table)
	rows, err := source.QueryContext(ctx, query)
	if err != nil {
		return TableResult{}, fmt.Errorf("读取 MySQL %s: %w", table, err)
	}
	defer rows.Close()
	insertSQL := "INSERT INTO " + quoteSQLiteIdentifier(table) + "(" +
		strings.Join(quotedSQLiteColumns, ",") + ") VALUES(" + strings.Join(placeholders, ",") + ")"
	statement, err := target.PrepareContext(ctx, insertSQL)
	if err != nil {
		return TableResult{}, fmt.Errorf("准备写入 SQLite %s: %w", table, err)
	}
	defer statement.Close()

	var copied int64
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return TableResult{}, fmt.Errorf("扫描 MySQL %s 第 %d 行: %w", table, copied+1, err)
		}
		for index, value := range values {
			if bytes, ok := value.([]byte); ok {
				values[index] = string(bytes)
			}
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return TableResult{}, fmt.Errorf("写入 SQLite %s 第 %d 行: %w", table, copied+1, err)
		}
		copied++
	}
	if err := rows.Err(); err != nil {
		return TableResult{}, fmt.Errorf("遍历 MySQL %s: %w", table, err)
	}

	// 3. 在同一目标事务内核对源读取量和目标最终行数。
	targetRows, err := countSQLiteRows(ctx, target, table)
	if err != nil {
		return TableResult{}, fmt.Errorf("统计 SQLite %s: %w", table, err)
	}
	if copied != targetRows {
		return TableResult{}, fmt.Errorf("核对 %s 行数: MySQL=%d SQLite=%d", table, copied, targetRows)
	}
	result := TableResult{
		Name: table, SourceRows: copied, TargetRows: targetRows,
		KeyColumns: append([]string(nil), spec.keyColumns...), DateColumn: spec.dateColumn,
	}
	if err := auditTable(ctx, source, target, spec, columns, &result); err != nil {
		return TableResult{}, err
	}
	return result, nil
}

// auditTable 核对单表日期范围和关键字段首尾样本。
// 输入：ctx 控制查询，source 和 target 是同批事务，spec 描述关键字段，columns 是完整字段，result 接收报告。
// 输出：全部核对一致返回 nil；范围或样本不同返回错误。
// 副作用：只读 MySQL 和 SQLite 事务，并修改 result。
func auditTable(
	ctx context.Context,
	source, target *sql.Tx,
	spec tableSpec,
	columns []string,
	result *TableResult,
) error {
	// 1. 空表没有样本；有日期字段时仍核对空范围。
	if spec.dateColumn != "" {
		sourceMin, sourceMax, err := valueRange(ctx, source, spec.name, spec.dateColumn, quoteMySQLIdentifier)
		if err != nil {
			return fmt.Errorf("读取 MySQL %s 日期范围: %w", spec.name, err)
		}
		targetMin, targetMax, err := valueRange(ctx, target, spec.name, spec.dateColumn, quoteSQLiteIdentifier)
		if err != nil {
			return fmt.Errorf("读取 SQLite %s 日期范围: %w", spec.name, err)
		}
		if sourceMin != targetMin || sourceMax != targetMax {
			return fmt.Errorf("核对 %s 日期范围: MySQL=%s~%s SQLite=%s~%s",
				spec.name, displayCanonicalValue(sourceMin), displayCanonicalValue(sourceMax),
				displayCanonicalValue(targetMin), displayCanonicalValue(targetMax))
		}
		result.DateMin, result.DateMax = displayCanonicalValue(sourceMin), displayCanonicalValue(sourceMax)
	}
	if result.SourceRows == 0 || len(spec.keyColumns) == 0 {
		return nil
	}

	// 2. 分别核对按关键字段正序和倒序取得的首尾完整记录。
	for _, descending := range []bool{false, true} {
		sourceValues, err := sampleRow(
			ctx, source, spec.name, columns, spec.keyColumns, descending, quoteMySQLIdentifier,
		)
		if err != nil {
			return fmt.Errorf("读取 MySQL %s 样本: %w", spec.name, err)
		}
		targetValues, err := sampleRow(
			ctx, target, spec.name, columns, spec.keyColumns, descending, quoteSQLiteIdentifier,
		)
		if err != nil {
			return fmt.Errorf("读取 SQLite %s 样本: %w", spec.name, err)
		}
		if !reflect.DeepEqual(sourceValues, targetValues) {
			direction := "首"
			if descending {
				direction = "尾"
			}
			return fmt.Errorf("核对 %s %s样本不一致", spec.name, direction)
		}
		result.Samples++
	}
	return nil
}

type identifierQuoter func(string) string

// valueRange 读取指定日期字段的最小值和最大值。
// 输入：ctx 控制查询，queryer 是源或目标事务，table 和 column 来自固定清单，quote 适配数据库方言。
// 输出：返回规范化后的最小和最大值。
// 副作用：只读数据库。
func valueRange(
	ctx context.Context,
	queryer queryRower,
	table, column string,
	quote identifierQuoter,
) (string, string, error) {
	// 1. 使用固定标识符构造聚合并规范驱动返回类型。
	query := "SELECT MIN(" + quote(column) + "),MAX(" + quote(column) + ") FROM " + quote(table)
	var minimum, maximum any
	if err := queryer.QueryRowContext(ctx, query).Scan(&minimum, &maximum); err != nil {
		return "", "", err
	}
	return canonicalValue(minimum), canonicalValue(maximum), nil
}

// sampleRow 读取按关键字段排序的一条完整记录。
// 输入：ctx 控制查询，queryer 是事务，table、columns、keys 来自固定清单，descending 控制首尾，quote 适配方言。
// 输出：返回每个字段的规范值；查询失败或空表返回错误。
// 副作用：只读数据库。
func sampleRow(
	ctx context.Context,
	queryer queryRower,
	table string,
	columns, keys []string,
	descending bool,
	quote identifierQuoter,
) ([]string, error) {
	// 1. 构造完整字段和稳定关键字段排序。
	quotedColumns := make([]string, len(columns))
	for index, column := range columns {
		quotedColumns[index] = quote(column)
	}
	order := make([]string, len(keys))
	for index, key := range keys {
		order[index] = quote(key)
		if descending {
			order[index] += " DESC"
		}
	}
	query := "SELECT " + strings.Join(quotedColumns, ",") + " FROM " + quote(table) +
		" ORDER BY " + strings.Join(order, ",") + " LIMIT 1"

	// 2. 扫描并把数值、文本、字节和空值统一成可比较形式。
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if err := queryer.QueryRowContext(ctx, query).Scan(destinations...); err != nil {
		return nil, err
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = canonicalValue(value)
	}
	return result, nil
}

// canonicalValue 统一 MySQL 与 SQLite 驱动的等价单元格表示。
// 输入：value 是任一驱动扫描出的基础值。
// 输出：空值、数值和文本分别返回稳定可比较字符串。
// 副作用：无。
func canonicalValue(value any) string {
	// 1. 先把驱动字节和基础类型转换为原始文本。
	if value == nil {
		return "<NULL>"
	}
	var text string
	switch typed := value.(type) {
	case []byte:
		text = string(typed)
	case string:
		text = typed
	default:
		text = fmt.Sprint(typed)
	}

	// 2. 数值使用有理数消除 100.00、100 和浮点扫描类型差异。
	if number, ok := new(big.Rat).SetString(text); ok {
		return "<NUMBER>" + number.RatString()
	}
	return "<TEXT>" + text
}

// displayCanonicalValue 把内部比较值转换为迁移报告文本。
// 输入：value 是 canonicalValue 返回的带类型前缀文本。
// 输出：返回适合人工核对的原始值，空值显示为空。
// 副作用：无。
func displayCanonicalValue(value string) string {
	// 1. 去除内部类型前缀并把空值压缩为空文本。
	if value == "<NULL>" {
		return ""
	}
	value = strings.TrimPrefix(value, "<TEXT>")
	return strings.TrimPrefix(value, "<NUMBER>")
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// countMySQLRows 统计 MySQL 白名单表行数。
// 输入：ctx 控制查询，queryer 是事务，table 是固定表名。
// 输出：返回行数；查询失败时返回错误。
// 副作用：只读旧 MySQL。
func countMySQLRows(ctx context.Context, queryer queryRower, table string) (int64, error) {
	// 1. 表名来自内部固定清单，使用 MySQL 标识符规则执行统计。
	var count int64
	if err := queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteMySQLIdentifier(table)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// countSQLiteRows 统计 SQLite 白名单表行数。
// 输入：ctx 控制查询，queryer 是事务，table 是固定表名。
// 输出：返回行数；查询失败时返回错误。
// 副作用：只读目标 SQLite。
func countSQLiteRows(ctx context.Context, queryer queryRower, table string) (int64, error) {
	// 1. 表名来自内部固定清单，使用 SQLite 标识符规则执行统计。
	var count int64
	if err := queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteSQLiteIdentifier(table)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// sqliteTableColumns 读取 SQLite 表的正式字段顺序。
// 输入：ctx 控制查询，tx 是目标事务，table 是固定表名。
// 输出：返回字段名列表；查询或扫描失败时返回错误。
// 副作用：只读 SQLite schema。
func sqliteTableColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	// 1. 使用 PRAGMA table_info 读取迁移后的目标结构。
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+quoteSQLiteIdentifier(table)+")")
	if err != nil {
		return nil, fmt.Errorf("读取 SQLite %s 字段: %w", table, err)
	}
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("扫描 SQLite %s 字段: %w", table, err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 SQLite %s 字段: %w", table, err)
	}
	return columns, nil
}

// quoteMySQLIdentifier 转义内部固定的 MySQL 标识符。
// 输入：value 是表名或字段名。
// 输出：返回反引号包裹的安全标识符。
// 副作用：无。
func quoteMySQLIdentifier(value string) string {
	// 1. 双写反引号后包裹标识符。
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

// quoteSQLiteIdentifier 转义内部固定的 SQLite 标识符。
// 输入：value 是表名或字段名。
// 输出：返回双引号包裹的安全标识符。
// 副作用：无。
func quoteSQLiteIdentifier(value string) string {
	// 1. 双写双引号后包裹标识符。
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
