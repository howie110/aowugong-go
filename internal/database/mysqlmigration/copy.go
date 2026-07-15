package mysqlmigration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const sqliteVariableBudget = 30000

// TablePlan 描述源表到目标表的一一字段复制计划。
type TablePlan struct {
	Name          string
	SourceColumns []string
	TargetColumns []string
	SourceTypes   []string
	PrimaryKey    string
	RangeColumns  []string
}

// ProgressFunc 接收单表累计复制行数，用于 CLI 日志。
type ProgressFunc func(table string, copied int64)

// copyTable 清空目标表后按批次复制源数据并保留主键。
// 输入：ctx 控制执行，source/target 是数据库，plan 是字段计划，batchSize 是批次，progress 是进度回调。
// 输出：返回复制行数；查询、转换或写入失败时返回错误。
// 副作用：只读源数据库，删除并重写目标 SQLite 指定表。
func copyTable(ctx context.Context, source SourceReader, target *sql.DB, plan TablePlan, batchSize int, progress ProgressFunc) (int64, error) {
	// 1. 校验标识符、字段映射和有效批次大小。
	if err := validatePlan(plan); err != nil {
		return 0, err
	}
	if batchSize < 1 {
		return 0, fmt.Errorf("迁移批次必须大于零")
	}
	maxRows := sqliteVariableBudget / len(plan.TargetColumns)
	if batchSize > maxRows {
		batchSize = maxRows
	}

	// 2. 在独立事务中清空旧目标数据，使完整重跑保持幂等。
	transaction, err := target.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始清空目标表 %s: %w", plan.Name, err)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(plan.Name)); err != nil {
		_ = transaction.Rollback()
		return 0, fmt.Errorf("清空目标表 %s: %w", plan.Name, err)
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("提交清空目标表 %s: %w", plan.Name, err)
	}

	// 3. 使用单条只读结果流避免大表反复跨网络分页，再按批次写入 SQLite。
	return streamTable(ctx, source, target, plan, batchSize, progress)
}

// streamTable 流式扫描一张源表并分批写入目标 SQLite。
// 输入：ctx 控制执行，source/target 是数据库，plan 是计划，batchSize 是写入批次，progress 接收进度。
// 输出：返回累计复制行数；查询、扫描或写入失败时返回错误。
// 副作用：只读源表并分批写入目标 SQLite。
func streamTable(ctx context.Context, source SourceReader, target *sql.DB, plan TablePlan, batchSize int, progress ProgressFunc) (int64, error) {
	// 1. 构造一次性有序源查询，主键表按主键，无主键表按全部字段稳定排序。
	query := "SELECT " + quoteIdentifiers(plan.SourceColumns) + " FROM " + quoteIdentifier(plan.Name)
	if plan.PrimaryKey == "" {
		query += " ORDER BY " + quoteIdentifiers(plan.SourceColumns)
	} else {
		query += " ORDER BY " + quoteIdentifier(plan.PrimaryKey)
	}
	rows, err := source.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("流式读取源表 %s: %w", plan.Name, err)
	}
	defer rows.Close()

	// 2. 动态扫描和规范化字段，达到批次上限立即提交目标事务。
	batch := make([][]any, 0, batchSize)
	var copied int64
	for rows.Next() {
		values := make([]any, len(plan.SourceColumns))
		pointers := make([]any, len(values))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return copied, fmt.Errorf("扫描源表 %s: %w", plan.Name, err)
		}
		for index := range values {
			sourceType := ""
			if index < len(plan.SourceTypes) {
				sourceType = plan.SourceTypes[index]
			}
			values[index] = normalizeValueForType(values[index], sourceType)
		}
		batch = append(batch, values)
		if len(batch) == batchSize {
			if err := insertBatch(ctx, target, plan, batch); err != nil {
				return copied, err
			}
			copied += int64(len(batch))
			batch = make([][]any, 0, batchSize)
			if progress != nil {
				progress(plan.Name, copied)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return copied, fmt.Errorf("遍历源表 %s: %w", plan.Name, err)
	}

	// 3. 提交不足一个完整批次的尾部数据。
	if len(batch) > 0 {
		if err := insertBatch(ctx, target, plan, batch); err != nil {
			return copied, err
		}
		copied += int64(len(batch))
		if progress != nil {
			progress(plan.Name, copied)
		}
	}
	return copied, nil
}

// insertBatch 在单个 SQLite 事务中写入一个多行批次。
// 输入：ctx 控制写入，target 是目标库，plan 是字段映射，batch 是行数据。
// 输出：成功返回 nil，准备或写入失败时返回错误。
// 副作用：向目标 SQLite 指定表新增一批数据。
func insertBatch(ctx context.Context, target *sql.DB, plan TablePlan, batch [][]any) error {
	// 1. 开启单批次事务，并选择兼顾解析和调用成本的一百行子批次。
	transaction, err := target.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始写入目标表 %s: %w", plan.Name, err)
	}
	chunkSize := 100
	if len(batch) < chunkSize {
		chunkSize = len(batch)
	}
	query := multiRowInsertSQL(plan, chunkSize)
	statement, err := transaction.PrepareContext(ctx, query)
	if err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("准备写入目标表 %s: %w", plan.Name, err)
	}
	defer statement.Close()

	// 2. 复用完整子批次语句，避免逐行 Exec 和超长 SQL 两个极端。
	index := 0
	for ; index+chunkSize <= len(batch); index += chunkSize {
		arguments, err := flattenRows(plan, batch[index:index+chunkSize], index)
		if err != nil {
			_ = transaction.Rollback()
			return err
		}
		if _, err := statement.ExecContext(ctx, arguments...); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("写入目标表 %s 第 %d 至 %d 行: %w", plan.Name, index+1, index+chunkSize, err)
		}
	}

	// 3. 使用匹配占位符数量的独立语句写入尾部不足一百行的数据。
	if index < len(batch) {
		tail := batch[index:]
		arguments, err := flattenRows(plan, tail, index)
		if err != nil {
			_ = transaction.Rollback()
			return err
		}
		if _, err := transaction.ExecContext(ctx, multiRowInsertSQL(plan, len(tail)), arguments...); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("写入目标表 %s 尾部批次: %w", plan.Name, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("提交目标表 %s 批次: %w", plan.Name, err)
	}
	return nil
}

// multiRowInsertSQL 构造固定行数的参数化 SQLite INSERT。
// 输入：plan 提供目标表和字段，rowCount 是本语句行数。
// 输出：返回只含已校验标识符和问号占位符的 SQL。
// 副作用：无。
func multiRowInsertSQL(plan TablePlan, rowCount int) string {
	// 1. 构造单行占位符并按行数重复。
	rowPlaceholder := "(" + strings.TrimRight(strings.Repeat("?,", len(plan.TargetColumns)), ",") + ")"
	return "INSERT INTO " + quoteIdentifier(plan.Name) + " (" + quoteIdentifiers(plan.TargetColumns) +
		") VALUES " + strings.TrimRight(strings.Repeat(rowPlaceholder+",", rowCount), ",")
}

// flattenRows 校验一组行并展平成 database/sql 参数。
// 输入：plan 提供字段数，rows 是子批次，startIndex 用于错误定位。
// 输出：返回按行字段顺序排列的参数；字段数不符时返回错误。
// 副作用：无。
func flattenRows(plan TablePlan, rows [][]any, startIndex int) ([]any, error) {
	// 1. 逐行检查字段数量并追加参数。
	arguments := make([]any, 0, len(rows)*len(plan.TargetColumns))
	for index, row := range rows {
		if len(row) != len(plan.TargetColumns) {
			return nil, fmt.Errorf("目标表 %s 第 %d 行字段数量不匹配", plan.Name, startIndex+index+1)
		}
		arguments = append(arguments, row...)
	}
	return arguments, nil
}

// normalizeValueForType 按源字段类型规范化 DATE 和其他驱动值。
// 输入：value 是扫描值，sourceType 是 MySQL data_type。
// 输出：DATE 返回 YYYY-MM-DD，其他值走通用转换。
// 副作用：无。
func normalizeValueForType(value any, sourceType string) any {
	// 1. DATE 类型必须去掉零点时间，保持页面和查询的日期契约。
	if date, ok := value.(time.Time); ok && strings.EqualFold(sourceType, "date") {
		return date.Format("2006-01-02")
	}
	return normalizeValue(value)
}

// normalizeValue 把 MySQL 驱动值转换为 SQLite 可预测的标量。
// 输入：value 是 database/sql 扫描结果。
// 输出：字节转 UTF-8 文本，时间转数据库时间文本，其他类型原样返回。
// 副作用：无。
func normalizeValue(value any) any {
	// 1. 统一 MySQL 字符、JSON、DECIMAL 和日期时间的驱动表示。
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format("2006-01-02 15:04:05")
	default:
		return value
	}
}

// validatePlan 校验表名、字段名和主键都可安全用于 SQL 标识符。
// 输入：plan 是复制计划。
// 输出：有效返回 nil，否则返回具体错误。
// 副作用：无。
func validatePlan(plan TablePlan) error {
	// 1. 检查表名和一一字段映射。
	if !validIdentifier(plan.Name) || len(plan.SourceColumns) == 0 || len(plan.SourceColumns) != len(plan.TargetColumns) {
		return fmt.Errorf("表 %q 迁移字段计划无效", plan.Name)
	}
	for index := range plan.SourceColumns {
		if !validIdentifier(plan.SourceColumns[index]) || !validIdentifier(plan.TargetColumns[index]) {
			return fmt.Errorf("表 %s 包含无效字段标识符", plan.Name)
		}
	}
	if plan.PrimaryKey != "" && indexOf(plan.SourceColumns, plan.PrimaryKey) < 0 {
		return fmt.Errorf("表 %s 主键不在源字段中", plan.Name)
	}
	return nil
}

// validIdentifier 检查 SQL 标识符只包含 ASCII 字母、数字和下划线。
// 输入：value 是表名或字段名。
// 输出：安全时返回 true。
// 副作用：无。
func validIdentifier(value string) bool {
	// 1. 拒绝空值、首字符数字和任何特殊字符。
	if value == "" || (value[0] >= '0' && value[0] <= '9') {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

// quoteIdentifier 使用反引号引用已校验 SQL 标识符。
// 输入：value 是安全表名或字段名。
// 输出：返回可用于 MySQL 和 SQLite 的引用标识符。
// 副作用：无。
func quoteIdentifier(value string) string {
	// 1. 调用方已校验，统一使用两种数据库都支持的反引号。
	return "`" + value + "`"
}

// quoteIdentifiers 批量引用并连接字段名。
// 输入：values 是字段名列表。
// 输出：返回逗号分隔的引用字段。
// 副作用：无。
func quoteIdentifiers(values []string) string {
	// 1. 保持字段顺序逐项引用。
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = quoteIdentifier(value)
	}
	return strings.Join(quoted, ",")
}

// indexOf 返回字符串在列表中的首个下标。
// 输入：values 是列表，target 是目标值。
// 输出：找到返回下标，否则返回 -1。
// 副作用：无。
func indexOf(values []string, target string) int {
	// 1. 按原顺序查找精确匹配。
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}
