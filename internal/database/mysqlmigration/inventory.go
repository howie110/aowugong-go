package mysqlmigration

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ColumnInventory 描述 MySQL 字段类型、可空性、主键信息和顺序。
type ColumnInventory struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	ColumnType string `json:"column_type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Position   int    `json:"position"`
}

// IndexInventory 描述 MySQL 索引及按顺序排列的字段。
type IndexInventory struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Columns []string `json:"columns"`
}

// TableInventory 描述一张源表的字段、索引、精确行数和特殊类型字段。
type TableInventory struct {
	Name        string            `json:"name"`
	RowCount    int64             `json:"row_count"`
	Columns     []ColumnInventory `json:"columns"`
	Indexes     []IndexInventory  `json:"indexes"`
	TimeColumns []string          `json:"time_columns"`
	JSONColumns []string          `json:"json_columns"`
	Migration   string            `json:"migration"`
	Reason      string            `json:"reason,omitempty"`
}

// Inventory 描述一次只读 MySQL 盘点结果。
type Inventory struct {
	Schema      string           `json:"schema"`
	GeneratedAt string           `json:"generated_at"`
	Tables      []TableInventory `json:"tables"`
	TotalRows   int64            `json:"total_rows"`
}

// InspectSource 只读盘点 MySQL 全部基础表、字段、精确行数、时间/JSON 和索引。
// 输入：ctx 控制查询，source 是 MySQL 连接，schema 是数据库名。
// 输出：返回按表名排序的完整盘点；任一元数据或计数查询失败时返回错误。
// 副作用：只读 MySQL information_schema 和全部源表 COUNT，不写源库。
func InspectSource(ctx context.Context, source SourceReader, schema string) (Inventory, error) {
	// 1. 校验库名并读取全部基础表和字段元数据。
	if source == nil || !validIdentifier(schema) {
		return Inventory{}, fmt.Errorf("MySQL 连接或数据库名无效")
	}
	rows, err := source.QueryContext(ctx, `SELECT table_name, column_name, data_type, column_type,
		is_nullable, column_key, ordinal_position
		FROM information_schema.columns WHERE table_schema = ?
		ORDER BY table_name, ordinal_position`, schema)
	if err != nil {
		return Inventory{}, fmt.Errorf("盘点 MySQL 字段: %w", err)
	}
	tableMap := make(map[string]*TableInventory)
	for rows.Next() {
		var tableName, columnName, dataType, columnType, nullable, columnKey string
		var position int
		if err := rows.Scan(&tableName, &columnName, &dataType, &columnType, &nullable, &columnKey, &position); err != nil {
			rows.Close()
			return Inventory{}, fmt.Errorf("扫描 MySQL 字段: %w", err)
		}
		table := tableMap[tableName]
		if table == nil {
			table = &TableInventory{Name: tableName, Migration: migrationStatus(tableName)}
			if reason, exists := HistoricalTables()[tableName]; exists {
				table.Reason = reason
			}
			tableMap[tableName] = table
		}
		column := ColumnInventory{
			Name: columnName, DataType: strings.ToLower(dataType), ColumnType: columnType,
			Nullable: nullable == "YES", PrimaryKey: columnKey == "PRI", Position: position,
		}
		table.Columns = append(table.Columns, column)
		if isTimeType(column.DataType) {
			table.TimeColumns = append(table.TimeColumns, columnName)
		}
		if column.DataType == "json" {
			table.JSONColumns = append(table.JSONColumns, columnName)
		}
	}
	if err := rows.Close(); err != nil {
		return Inventory{}, fmt.Errorf("关闭 MySQL 字段结果: %w", err)
	}
	if err := rows.Err(); err != nil {
		return Inventory{}, fmt.Errorf("遍历 MySQL 字段: %w", err)
	}

	// 2. 一次读取全部索引并按表和索引字段顺序聚合。
	indexRows, err := source.QueryContext(ctx, `SELECT table_name, index_name, non_unique, column_name
		FROM information_schema.statistics WHERE table_schema = ?
		ORDER BY table_name, index_name, seq_in_index`, schema)
	if err != nil {
		return Inventory{}, fmt.Errorf("盘点 MySQL 索引: %w", err)
	}
	indexPositions := make(map[string]map[string]int)
	for indexRows.Next() {
		var tableName, indexName, columnName string
		var nonUnique int
		if err := indexRows.Scan(&tableName, &indexName, &nonUnique, &columnName); err != nil {
			indexRows.Close()
			return Inventory{}, fmt.Errorf("扫描 MySQL 索引: %w", err)
		}
		table := tableMap[tableName]
		if table == nil {
			continue
		}
		if indexPositions[tableName] == nil {
			indexPositions[tableName] = make(map[string]int)
		}
		position, exists := indexPositions[tableName][indexName]
		if !exists {
			position = len(table.Indexes)
			indexPositions[tableName][indexName] = position
			table.Indexes = append(table.Indexes, IndexInventory{Name: indexName, Unique: nonUnique == 0})
		}
		table.Indexes[position].Columns = append(table.Indexes[position].Columns, columnName)
	}
	if err := indexRows.Close(); err != nil {
		return Inventory{}, fmt.Errorf("关闭 MySQL 索引结果: %w", err)
	}
	if err := indexRows.Err(); err != nil {
		return Inventory{}, fmt.Errorf("遍历 MySQL 索引: %w", err)
	}

	// 3. 对每张表执行精确 COUNT 并构建稳定排序结果。
	names := make([]string, 0, len(tableMap))
	for name := range tableMap {
		names = append(names, name)
	}
	sort.Strings(names)
	result := Inventory{Schema: schema, GeneratedAt: time.Now().Format(time.RFC3339), Tables: make([]TableInventory, 0, len(names))}
	for _, name := range names {
		var count int64
		if err := source.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(name)).Scan(&count); err != nil {
			return Inventory{}, fmt.Errorf("统计 MySQL 表 %s 行数: %w", name, err)
		}
		tableMap[name].RowCount = count
		result.TotalRows += count
		result.Tables = append(result.Tables, *tableMap[name])
	}
	return result, nil
}

// isTimeType 判断 MySQL 字段是否属于日期或时间类型。
// 输入：dataType 是 information_schema.data_type。
// 输出：日期时间类型返回 true。
// 副作用：无。
func isTimeType(dataType string) bool {
	// 1. 覆盖当前源库使用及 MySQL 常见时间类型。
	switch strings.ToLower(dataType) {
	case "date", "datetime", "timestamp", "time", "year":
		return true
	default:
		return false
	}
}

// migrationStatus 返回源表应迁移或明确跳过的盘点状态。
// 输入：name 是源表名。
// 输出：有效表返回 migrate，历史表返回 skip，未知表返回 review。
// 副作用：无。
func migrationStatus(name string) string {
	// 1. 按代码维护的审计边界分类。
	for _, spec := range DefaultTableSpecs() {
		if spec.Name == name {
			return "migrate"
		}
	}
	if _, exists := HistoricalTables()[name]; exists {
		return "skip"
	}
	return "review"
}
