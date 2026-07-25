// Package databaseview 提供管理员只读查看 SQLite 表结构和数据的能力。
package databaseview

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxExportRows = 100000

var (
	// ErrTableNotFound 表示请求的表不属于当前 SQLite 应用表。
	ErrTableNotFound = errors.New("数据表不存在")
	// ErrInvalidPagination 表示分页参数超出只读页面允许范围。
	ErrInvalidPagination = errors.New("分页参数无效")
	// ErrSearchTooLong 表示搜索文本超过只读页面限制。
	ErrSearchTooLong = errors.New("搜索内容不能超过 100 个字符")
	// ErrExportTooLarge 表示筛选结果超过单次导出上限。
	ErrExportTooLarge = errors.New("导出数据超过十万行上限")
)

// TableSummary 描述只读页面中的单张 SQLite 表。
type TableSummary struct {
	Name        string `json:"name"`
	RowCount    int64  `json:"row_count"`
	ColumnCount int    `json:"column_count"`
}

// Summary 描述 SQLite 文件与全部应用表概况。
type Summary struct {
	Engine      string         `json:"engine"`
	JournalMode string         `json:"journal_mode"`
	SizeBytes   int64          `json:"size_bytes"`
	TableCount  int            `json:"table_count"`
	TotalRows   int64          `json:"total_rows"`
	Tables      []TableSummary `json:"tables"`
}

// Column 描述 SQLite 表字段及页面展示约束。
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	NotNull    bool   `json:"not_null"`
	PrimaryKey bool   `json:"primary_key"`
	Sensitive  bool   `json:"sensitive"`
}

// RowsPage 描述一页只读表数据。
type RowsPage struct {
	Table    string           `json:"table"`
	Columns  []Column         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// Service 提供严格白名单化的 SQLite 只读查询。
type Service struct {
	db *sql.DB
}

// NewService 创建 SQLite 只读查看服务。
// 输入：db 是已经完成版本迁移的应用 SQLite 连接池。
// 输出：返回可供 HTTP 层复用的只读服务。
// 副作用：无，不立即查询数据库。
func NewService(db *sql.DB) *Service {
	// 1. 保存显式注入的数据库连接。
	return &Service{db: db}
}

// Summary 统计 SQLite 文件参数和全部应用表行数。
// 输入：ctx 控制只读查询生命周期。
// 输出：返回数据库大小、日志模式和按表名排序的表概况。
// 副作用：只读 SQLite schema、PRAGMA 和表行数。
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	// 1. 读取 SQLite 文件页参数和 WAL 模式。
	if s == nil || s.db == nil {
		return Summary{}, fmt.Errorf("数据库只读服务未初始化")
	}
	var pageCount, pageSize int64
	var journalMode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return Summary{}, fmt.Errorf("读取 SQLite page_count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return Summary{}, fmt.Errorf("读取 SQLite page_size: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return Summary{}, fmt.Errorf("读取 SQLite journal_mode: %w", err)
	}

	// 2. 读取全部业务表并逐表统计行数和字段数。
	names, err := s.tableNames(ctx)
	if err != nil {
		return Summary{}, err
	}
	result := Summary{
		Engine: "SQLite", JournalMode: strings.ToUpper(journalMode),
		SizeBytes: pageCount * pageSize, TableCount: len(names),
		Tables: make([]TableSummary, 0, len(names)),
	}
	for _, name := range names {
		columns, err := s.columns(ctx, name)
		if err != nil {
			return Summary{}, err
		}
		count, err := s.countRows(ctx, name, "", columns)
		if err != nil {
			return Summary{}, err
		}
		result.Tables = append(result.Tables, TableSummary{
			Name: name, RowCount: count, ColumnCount: len(columns),
		})
		result.TotalRows += count
	}
	return result, nil
}

// Rows 读取指定 SQLite 表的一页数据。
// 输入：ctx 控制查询，table 是现有业务表，search 是可选文本，page 和 pageSize 控制分页。
// 输出：返回字段定义、总数和当前页；表不存在或查询失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Rows(ctx context.Context, table, search string, page, pageSize int) (RowsPage, error) {
	// 1. 校验分页并从 schema 白名单确认表和字段。
	if page < 1 || pageSize < 1 || pageSize > 200 {
		return RowsPage{}, ErrInvalidPagination
	}
	table, columns, err := s.validatedTable(ctx, table)
	if err != nil {
		return RowsPage{}, err
	}
	search = strings.TrimSpace(search)
	if len([]rune(search)) > 100 {
		return RowsPage{}, ErrSearchTooLong
	}

	// 2. 构造参数化搜索、总数和稳定倒序分页查询。
	whereSQL, searchArgs := buildSearch(columns, search)
	total, err := s.countRows(ctx, table, whereSQL, columns, searchArgs...)
	if err != nil {
		return RowsPage{}, err
	}
	orderSQL := buildOrder(columns)
	query := "SELECT * FROM " + quoteIdentifier(table) + whereSQL + orderSQL + " LIMIT ? OFFSET ?"
	args := append(append([]any{}, searchArgs...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return RowsPage{}, fmt.Errorf("读取 SQLite 表 %s: %w", table, err)
	}
	defer rows.Close()

	// 3. 按正式字段顺序扫描并隐藏敏感字段。
	items, err := scanRows(rows, columns)
	if err != nil {
		return RowsPage{}, fmt.Errorf("扫描 SQLite 表 %s: %w", table, err)
	}
	return RowsPage{
		Table: table, Columns: columns, Rows: items,
		Total: total, Page: page, PageSize: pageSize,
	}, nil
}

// ExportCSV 流式导出指定 SQLite 表的只读筛选结果。
// 输入：ctx 控制查询，table 是现有业务表，search 是可选文本，output 接收 CSV。
// 输出：成功返回 nil；超过十万行、输出中断或查询失败时返回错误。
// 副作用：只读 SQLite，并向 output 逐行写入带 UTF-8 BOM 的脱敏 CSV。
func (s *Service) ExportCSV(ctx context.Context, table, search string, output io.Writer) error {
	// 1. 校验表和搜索长度，并在读取前限制导出规模。
	if output == nil {
		return fmt.Errorf("CSV 输出不能为空")
	}
	table, columns, err := s.validatedTable(ctx, table)
	if err != nil {
		return err
	}
	search = strings.TrimSpace(search)
	if len([]rune(search)) > 100 {
		return ErrSearchTooLong
	}
	whereSQL, args := buildSearch(columns, search)
	total, err := s.countRows(ctx, table, whereSQL, columns, args...)
	if err != nil {
		return err
	}
	if total > maxExportRows {
		return ErrExportTooLarge
	}

	// 2. 按页面相同排序读取全部匹配行。
	rows, err := s.db.QueryContext(ctx,
		"SELECT * FROM "+quoteIdentifier(table)+whereSQL+buildOrder(columns), args...)
	if err != nil {
		return fmt.Errorf("导出 SQLite 表 %s: %w", table, err)
	}
	defer rows.Close()

	// 3. 写入字段头后逐行扫描和脱敏，避免把完整导出保存在内存。
	if _, err := io.WriteString(output, "\xef\xbb\xbf"); err != nil {
		return fmt.Errorf("写入 CSV BOM: %w", err)
	}
	writer := csv.NewWriter(output)
	headers := make([]string, len(columns))
	for index, column := range columns {
		headers[index] = column.Name
	}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("写入 CSV 字段: %w", err)
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return fmt.Errorf("扫描 SQLite 导出表 %s: %w", table, err)
		}
		record := make([]string, len(columns))
		for index, column := range columns {
			if column.Sensitive && values[index] != nil {
				record[index] = "已隐藏"
				continue
			}
			record[index] = exportValue(normalizeValue(values[index]))
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("写入 CSV 数据: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 SQLite 导出表 %s: %w", table, err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("完成 CSV 导出: %w", err)
	}
	return nil
}

// validatedTable 确认请求表存在于 SQLite schema 并返回字段。
// 输入：ctx 控制查询，table 是用户请求的表名。
// 输出：返回规范表名和字段；不存在时返回 ErrTableNotFound。
// 副作用：只读 SQLite schema。
func (s *Service) validatedTable(ctx context.Context, table string) (string, []Column, error) {
	// 1. 精确匹配 schema 返回的应用表名，禁止把用户输入直接拼入 SQL。
	table = strings.TrimSpace(table)
	names, err := s.tableNames(ctx)
	if err != nil {
		return "", nil, err
	}
	index := sort.SearchStrings(names, table)
	if index >= len(names) || names[index] != table {
		return "", nil, ErrTableNotFound
	}
	columns, err := s.columns(ctx, table)
	if err != nil {
		return "", nil, err
	}
	return table, columns, nil
}

// tableNames 返回允许管理员查看的应用表名。
// 输入：ctx 控制 schema 查询。
// 输出：返回排除 SQLite 和 Goose 内部表后的有序名称。
// 副作用：只读 SQLite schema。
func (s *Service) tableNames(ctx context.Context) ([]string, error) {
	// 1. 只接受真正的数据表并排除框架内部元数据。
	rows, err := s.db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'goose_db_version'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("读取 SQLite 表清单: %w", err)
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("扫描 SQLite 表名: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 SQLite 表名: %w", err)
	}
	return names, nil
}

// columns 读取指定白名单表的字段结构。
// 输入：ctx 控制查询，table 是经过 schema 校验的表名。
// 输出：返回字段顺序、类型、约束和脱敏标记。
// 副作用：只读 SQLite schema。
func (s *Service) columns(ctx context.Context, table string) ([]Column, error) {
	// 1. 使用 PRAGMA table_info 读取正式字段顺序。
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return nil, fmt.Errorf("读取 SQLite 表 %s 字段: %w", table, err)
	}
	defer rows.Close()
	columns := make([]Column, 0)
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("扫描 SQLite 表 %s 字段: %w", table, err)
		}
		columns = append(columns, Column{
			Name: name, Type: kind, NotNull: notNull == 1,
			PrimaryKey: primaryKey > 0, Sensitive: isSensitiveColumn(name),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 SQLite 表 %s 字段: %w", table, err)
	}
	return columns, nil
}

// countRows 统计指定白名单表的筛选行数。
// 输入：ctx 控制查询，table 已校验，whereSQL 由内部构造，columns 用于保留函数契约，args 是搜索参数。
// 输出：返回匹配行数。
// 副作用：只读 SQLite。
func (s *Service) countRows(
	ctx context.Context,
	table string,
	whereSQL string,
	columns []Column,
	args ...any,
) (int64, error) {
	// 1. 保留字段参数用于强调 whereSQL 只可由当前模块按字段清单生成。
	_ = columns
	var count int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+quoteIdentifier(table)+whereSQL, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计 SQLite 表 %s: %w", table, err)
	}
	return count, nil
}

// buildSearch 按现有字段构造参数化模糊搜索。
// 输入：columns 是正式字段，search 是已限制长度的文本。
// 输出：返回内部 SQL 片段和重复绑定参数。
// 副作用：无。
func buildSearch(columns []Column, search string) (string, []any) {
	// 1. 空搜索不增加 WHERE 条件。
	if search == "" {
		return "", nil
	}

	// 2. 对每个非敏感字段执行文本转换后的包含匹配。
	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	pattern := "%" + escapeLike(search) + "%"
	for _, column := range columns {
		if column.Sensitive {
			continue
		}
		conditions = append(conditions,
			"CAST("+quoteIdentifier(column.Name)+" AS TEXT) LIKE ? ESCAPE '\\'")
		args = append(args, pattern)
	}
	if len(conditions) == 0 {
		return " WHERE 0 = 1", nil
	}
	return " WHERE (" + strings.Join(conditions, " OR ") + ")", args
}

// buildOrder 为白名单表构造稳定的倒序展示规则。
// 输入：columns 是正式字段顺序和主键信息。
// 输出：优先返回主键倒序，否则按 SQLite rowid 倒序。
// 副作用：无。
func buildOrder(columns []Column) string {
	// 1. 复合主键按声明顺序共同倒序。
	primaryKeys := make([]string, 0)
	for _, column := range columns {
		if column.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteIdentifier(column.Name)+" DESC")
		}
	}
	if len(primaryKeys) > 0 {
		return " ORDER BY " + strings.Join(primaryKeys, ",")
	}
	return " ORDER BY rowid DESC"
}

// scanRows 把数据库行转换为前端可消费且已脱敏的对象。
// 输入：rows 是活动查询结果，columns 是正式字段顺序。
// 输出：返回字段名到规范值的映射列表。
// 副作用：消费 rows 游标。
func scanRows(rows *sql.Rows, columns []Column) ([]map[string]any, error) {
	// 1. 为每行建立动态扫描目标并按字段顺序读取。
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}

		// 2. 隐藏敏感字段并规范二进制、数字和空值。
		item := make(map[string]any, len(columns))
		for index, column := range columns {
			if column.Sensitive && values[index] != nil {
				item[column.Name] = "已隐藏"
				continue
			}
			item[column.Name] = normalizeValue(values[index])
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// normalizeValue 把 SQLite 驱动值转换为稳定 JSON 值。
// 输入：value 是数据库驱动返回的单元格值。
// 输出：文本二进制返回字符串，其他基础值保持原样。
// 副作用：无。
func normalizeValue(value any) any {
	// 1. 可读 UTF-8 字节直接转文本，其他字节使用 Base64 标记。
	bytesValue, ok := value.([]byte)
	if !ok {
		return value
	}
	if utf8.Valid(bytesValue) {
		return string(bytesValue)
	}
	return "base64:" + base64.StdEncoding.EncodeToString(bytesValue)
}

// exportValue 把规范单元格转换为 CSV 文本。
// 输入：value 是已规范和脱敏的单元格值。
// 输出：返回可写入 CSV 的文本。
// 副作用：无。
func exportValue(value any) string {
	// 1. 空值保留为空，其余基础类型使用稳定文本。
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return safeCSVText(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// safeCSVText 防止外部文本在表格软件中被解释为公式。
// 输入：value 是 SQLite 文本字段。
// 输出：公式前缀文本会增加单引号，其余文本原样返回。
// 副作用：无。
func safeCSVText(value string) string {
	// 1. Excel 类软件会把四种前缀和控制字符视为公式入口。
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
}

// isSensitiveColumn 判断字段是否包含身份凭据。
// 输入：name 是 SQLite 字段名。
// 输出：密码、令牌、密钥和密文类字段返回 true。
// 副作用：无。
func isSensitiveColumn(name string) bool {
	// 1. 按分词后的常见敏感标识匹配，避免把普通业务字段误判。
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, fragment := range []string{"password", "passwd", "secret", "token", "api_key", "access_key", "cipher"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// escapeLike 转义 SQLite LIKE 通配符。
// 输入：value 是用户搜索文本。
// 输出：返回只按字面匹配百分号、下划线和反斜杠的文本。
// 副作用：无。
func escapeLike(value string) string {
	// 1. 先转义转义符本身，再处理两个通配符。
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

// quoteIdentifier 转义 schema 白名单中的 SQLite 标识符。
// 输入：value 是已经校验过的表名或字段名。
// 输出：返回双引号包裹的 SQLite 标识符。
// 副作用：无。
func quoteIdentifier(value string) string {
	// 1. 双写双引号并包裹标识符。
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
