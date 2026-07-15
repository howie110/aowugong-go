package mysqlmigration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type targetColumn struct {
	name       string
	notNull    bool
	defaultSQL sql.NullString
	primaryKey bool
}

// BuildPlans 对照源盘点和已迁移 SQLite 表生成确定字段复制计划。
// 输入：ctx 控制查询，target 是 SQLite，inventory 是源盘点，specs 是有效表，selected 可筛选表。
// 输出：返回规格顺序的计划；表、必填字段或选择无效时返回错误。
// 副作用：只读 SQLite schema 元数据。
func BuildPlans(ctx context.Context, target *sql.DB, inventory Inventory, specs []TableSpec, selected []string) ([]TablePlan, error) {
	// 1. 建立源表、规格和可选筛选映射并拒绝未知选择。
	sourceTables := make(map[string]TableInventory, len(inventory.Tables))
	for _, table := range inventory.Tables {
		sourceTables[table.Name] = table
	}
	specMap := make(map[string]TableSpec, len(specs))
	for _, spec := range specs {
		specMap[spec.Name] = spec
	}
	selectedMap := make(map[string]bool)
	for _, name := range selected {
		name = strings.TrimSpace(name)
		if _, exists := specMap[name]; !exists {
			return nil, fmt.Errorf("选择了未审计迁移表 %s", name)
		}
		selectedMap[name] = true
	}

	// 2. 按规格顺序读取目标字段并映射源字段和类型。
	plans := make([]TablePlan, 0, len(specs))
	for _, spec := range specs {
		if len(selectedMap) > 0 && !selectedMap[spec.Name] {
			continue
		}
		sourceTable, exists := sourceTables[spec.Name]
		if !exists {
			return nil, fmt.Errorf("MySQL 缺少有效源表 %s", spec.Name)
		}
		targetColumns, err := inspectTargetColumns(ctx, target, spec.Name)
		if err != nil {
			return nil, err
		}
		sourceColumns := make(map[string]ColumnInventory, len(sourceTable.Columns))
		primaryColumns := make([]string, 0)
		for _, column := range sourceTable.Columns {
			sourceColumns[column.Name] = column
			if column.PrimaryKey {
				primaryColumns = append(primaryColumns, column.Name)
			}
		}
		ignoredSourceColumns := make(map[string]bool, len(spec.IgnoredSourceColumns))
		for _, column := range spec.IgnoredSourceColumns {
			if !validIdentifier(column) {
				return nil, fmt.Errorf("表 %s 包含无效忽略字段 %q", spec.Name, column)
			}
			if _, exists := sourceColumns[column]; !exists {
				return nil, fmt.Errorf("表 %s 声明忽略的源字段 %s 不存在", spec.Name, column)
			}
			ignoredSourceColumns[column] = true
		}
		plan := TablePlan{Name: spec.Name}
		for _, targetColumn := range targetColumns {
			sourceName := targetColumn.name
			if renamed, renamedExists := spec.ColumnRenames[targetColumn.name]; renamedExists {
				sourceName = renamed
			}
			sourceColumn, sourceExists := sourceColumns[sourceName]
			if !sourceExists {
				if targetColumn.notNull && !targetColumn.defaultSQL.Valid && !targetColumn.primaryKey {
					return nil, fmt.Errorf("目标表 %s 必填字段 %s 在 MySQL 中不存在且无默认值", spec.Name, targetColumn.name)
				}
				continue
			}
			plan.SourceColumns = append(plan.SourceColumns, sourceName)
			plan.TargetColumns = append(plan.TargetColumns, targetColumn.name)
			plan.SourceTypes = append(plan.SourceTypes, sourceColumn.DataType)
		}
		for _, sourceColumn := range sourceTable.Columns {
			if indexOf(plan.SourceColumns, sourceColumn.Name) < 0 && !ignoredSourceColumns[sourceColumn.Name] {
				return nil, fmt.Errorf("表 %s 的 MySQL 源字段 %s 未映射到 SQLite，也未显式忽略", spec.Name, sourceColumn.Name)
			}
		}
		if len(primaryColumns) == 1 && indexOf(plan.SourceColumns, primaryColumns[0]) >= 0 {
			plan.PrimaryKey = primaryColumns[0]
		}
		for _, rangeColumn := range spec.RangeColumns {
			if indexOf(plan.TargetColumns, rangeColumn) < 0 {
				return nil, fmt.Errorf("表 %s 的关键核验字段 %s 未纳入迁移计划", spec.Name, rangeColumn)
			}
			plan.RangeColumns = append(plan.RangeColumns, rangeColumn)
		}
		if err := validatePlan(plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// inspectTargetColumns 读取单张 SQLite 表的字段顺序、约束和默认值。
// 输入：ctx 控制查询，target 是 SQLite，table 是已校验规格表名。
// 输出：返回字段元数据；目标表不存在或查询失败时返回错误。
// 副作用：只读 SQLite PRAGMA 元数据。
func inspectTargetColumns(ctx context.Context, target *sql.DB, table string) ([]targetColumn, error) {
	// 1. 使用固定审计表名读取 PRAGMA table_info。
	if target == nil || !validIdentifier(table) {
		return nil, fmt.Errorf("目标 SQLite 或表名无效")
	}
	rows, err := target.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return nil, fmt.Errorf("读取目标表 %s 字段: %w", table, err)
	}
	defer rows.Close()
	result := make([]targetColumn, 0)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultSQL sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultSQL, &primaryKey); err != nil {
			return nil, fmt.Errorf("扫描目标表 %s 字段: %w", table, err)
		}
		result = append(result, targetColumn{
			name: name, notNull: notNull != 0, defaultSQL: defaultSQL, primaryKey: primaryKey != 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历目标表 %s 字段: %w", table, err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("目标 SQLite 缺少表 %s", table)
	}
	return result, nil
}

// UnknownSourceTables 返回既未迁移也未明确跳过的源表。
// 输入：inventory 是完整源盘点。
// 输出：返回未知表名列表；为空表示审计边界完整。
// 副作用：无。
func UnknownSourceTables(inventory Inventory) []string {
	// 1. 按盘点顺序收集 review 状态，阻止静默丢表。
	result := make([]string, 0)
	for _, table := range inventory.Tables {
		if table.Migration == "review" {
			result = append(result, table.Name)
		}
	}
	return result
}
