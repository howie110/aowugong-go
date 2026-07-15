package mysqlmigration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// RangeVerification 描述一个关键字段在源库和目标库的最小最大值。
type RangeVerification struct {
	SourceMin string `json:"source_min"`
	SourceMax string `json:"source_max"`
	TargetMin string `json:"target_min"`
	TargetMax string `json:"target_max"`
	Matched   bool   `json:"matched"`
}

// TableVerification 描述逐表行数、关键范围和抽样记录核验结果。
type TableVerification struct {
	Name            string                       `json:"name"`
	SourceCount     int64                        `json:"source_count"`
	TargetCount     int64                        `json:"target_count"`
	CountMatched    bool                         `json:"count_matched"`
	Ranges          map[string]RangeVerification `json:"ranges"`
	SamplesCompared int                          `json:"samples_compared"`
	SamplesMatched  bool                         `json:"samples_matched"`
	Passed          bool                         `json:"passed"`
}

// VerifyTables 逐表比较精确行数、关键字段范围和首尾抽样记录。
// 输入：ctx 控制查询，source/target 是数据库，plans 是字段与范围计划。
// 输出：返回每张表的核验报告；查询失败时返回错误，数据不一致通过 Passed 表示。
// 副作用：只读 MySQL 和 SQLite，不修改任何数据。
func VerifyTables(ctx context.Context, source SourceReader, target *sql.DB, plans []TablePlan) ([]TableVerification, error) {
	// 1. 按迁移计划逐表读取精确行数。
	results := make([]TableVerification, 0, len(plans))
	for _, plan := range plans {
		verification := TableVerification{Name: plan.Name, Ranges: make(map[string]RangeVerification)}
		if err := source.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(plan.Name)).Scan(&verification.SourceCount); err != nil {
			return nil, fmt.Errorf("核验源表 %s 行数: %w", plan.Name, err)
		}
		if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(plan.Name)).Scan(&verification.TargetCount); err != nil {
			return nil, fmt.Errorf("核验目标表 %s 行数: %w", plan.Name, err)
		}
		verification.CountMatched = verification.SourceCount == verification.TargetCount

		// 2. 比较规格维护的日期、代码或主键范围。
		rangesMatched := true
		for _, targetColumn := range plan.RangeColumns {
			index := indexOf(plan.TargetColumns, targetColumn)
			if index < 0 {
				continue
			}
			sourceColumn := plan.SourceColumns[index]
			sourceType := ""
			if index < len(plan.SourceTypes) {
				sourceType = plan.SourceTypes[index]
			}
			sourceMin, sourceMax, err := queryRange(ctx, source, plan.Name, sourceColumn, sourceType)
			if err != nil {
				return nil, err
			}
			targetMin, targetMax, err := queryRange(ctx, target, plan.Name, targetColumn, sourceType)
			if err != nil {
				return nil, err
			}
			item := RangeVerification{
				SourceMin: sourceMin, SourceMax: sourceMax, TargetMin: targetMin, TargetMax: targetMax,
				Matched: sourceMin == targetMin && sourceMax == targetMax,
			}
			verification.Ranges[targetColumn] = item
			if !item.Matched {
				rangesMatched = false
			}
		}

		// 3. 比较首尾各三行的全部迁移字段。
		sourceOrderColumns := plan.SourceColumns
		targetOrderColumns := plan.TargetColumns
		if plan.PrimaryKey != "" {
			primaryIndex := indexOf(plan.SourceColumns, plan.PrimaryKey)
			sourceOrderColumns = []string{plan.PrimaryKey}
			targetOrderColumns = []string{plan.TargetColumns[primaryIndex]}
		}
		sourceSamples, err := readSamples(ctx, source, plan.Name, plan.SourceColumns, plan.SourceTypes, sourceOrderColumns)
		if err != nil {
			return nil, fmt.Errorf("读取源表 %s 抽样: %w", plan.Name, err)
		}
		targetSamples, err := readSamples(ctx, target, plan.Name, plan.TargetColumns, plan.SourceTypes, targetOrderColumns)
		if err != nil {
			return nil, fmt.Errorf("读取目标表 %s 抽样: %w", plan.Name, err)
		}
		verification.SamplesCompared = len(sourceSamples)
		verification.SamplesMatched = stringSlicesEqual(sourceSamples, targetSamples)
		verification.Passed = verification.CountMatched && rangesMatched && verification.SamplesMatched
		results = append(results, verification)
	}
	return results, nil
}

// queryRange 读取单字段最小最大值并转换为可稳定比较的文本。
// 输入：ctx、db、table、column 和 sourceType 描述查询。
// 输出：返回规范化最小最大值；查询失败时返回错误。
// 副作用：只读指定数据库。
func queryRange(ctx context.Context, db SourceReader, table, column, sourceType string) (string, string, error) {
	// 1. 参数只接受迁移计划中的安全标识符。
	if !validIdentifier(table) || !validIdentifier(column) {
		return "", "", fmt.Errorf("核验范围标识符无效")
	}
	var minimum, maximum any
	query := "SELECT MIN(" + quoteIdentifier(column) + "), MAX(" + quoteIdentifier(column) + ") FROM " + quoteIdentifier(table)
	if err := db.QueryRowContext(ctx, query).Scan(&minimum, &maximum); err != nil {
		return "", "", fmt.Errorf("核验 %s.%s 范围: %w", table, column, err)
	}
	return canonicalValue(minimum, sourceType), canonicalValue(maximum, sourceType), nil
}

// readSamples 读取按稳定字段排序的首尾各三行并生成规范文本。
// 输入：ctx、db、table、columns、types 和 orderColumn 描述抽样。
// 输出：返回最多六条规范记录；查询失败时返回错误。
// 副作用：只读指定数据库。
func readSamples(ctx context.Context, db SourceReader, table string, columns, types, orderColumns []string) ([]string, error) {
	// 1. 无显式排序字段时使用全部迁移字段，随后读取正序和倒序样本。
	if len(orderColumns) == 0 {
		orderColumns = columns
	}
	result := make([]string, 0, 6)
	for _, direction := range []string{"ASC", "DESC"} {
		query := "SELECT " + quoteIdentifiers(columns) + " FROM " + quoteIdentifier(table) +
			" ORDER BY " + orderClause(orderColumns, direction) + " LIMIT 3"
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				return nil, err
			}
			canonical := make([]string, len(values))
			for index, value := range values {
				sourceType := ""
				if index < len(types) {
					sourceType = types[index]
				}
				canonical[index] = canonicalValue(value, sourceType)
			}
			encoded, err := json.Marshal(canonical)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result = append(result, string(encoded))
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// orderClause 为每个稳定排序字段应用相同升降序。
// 输入：columns 是字段列表，direction 是 ASC 或 DESC。
// 输出：返回可直接放入 ORDER BY 后的安全片段。
// 副作用：无。
func orderClause(columns []string, direction string) string {
	// 1. 逐字段引用并应用固定方向，避免方向只作用于最后一列。
	parts := make([]string, len(columns))
	for index, column := range columns {
		parts[index] = quoteIdentifier(column) + " " + direction
	}
	return strings.Join(parts, ",")
}

// canonicalValue 把源和目标标量转换为可跨驱动比较的文本。
// 输入：value 是扫描值，sourceType 是源 MySQL 类型。
// 输出：返回日期、数字、JSON 和普通文本的稳定表示。
// 副作用：无。
func canonicalValue(value any, sourceType string) string {
	// 1. 先应用数据库驱动值和 DATE 的统一转换。
	value = normalizeValueForType(value, sourceType)
	if value == nil {
		return "<NULL>"
	}
	text := fmt.Sprint(value)

	// 2. 数值按整数、精确小数和浮点分别规范，避免 BIGINT/DECIMAL 精度丢失。
	if number, ok := canonicalNumber(text, sourceType); ok {
		return number
	}

	// 3. JSON 解析后重编码，消除对象字段顺序和空白差异。
	if strings.EqualFold(sourceType, "json") {
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			if encoded, err := json.Marshal(parsed); err == nil {
				return string(encoded)
			}
		}
	}
	return text
}

// canonicalNumber 按 MySQL 数值类型生成不丢精度的规范文本。
// 输入：text 是驱动标量文本，sourceType 是 information_schema.data_type。
// 输出：可解析数值返回规范文本和 true，非数值或解析失败返回 false。
// 副作用：无。
func canonicalNumber(text, sourceType string) (string, bool) {
	// 1. 整数使用任意精度 Int，保留 BIGINT 的每一位。
	switch strings.ToLower(sourceType) {
	case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "bit":
		if number, ok := new(big.Int).SetString(strings.TrimSpace(text), 10); ok {
			return number.String(), true
		}
		return "", false
	case "decimal", "numeric":
		// 2. 定点小数使用任意精度 Rat，统一尾零但保留所有有效位。
		if number, ok := new(big.Rat).SetString(strings.TrimSpace(text)); ok {
			return number.RatString(), true
		}
		return "", false
	case "float", "double", "real":
		// 3. 原生浮点字段按 IEEE 754 双精度规范化。
		if number, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
			return strconv.FormatFloat(number, 'g', 17, 64), true
		}
		return "", false
	}
	return "", false
}

// stringSlicesEqual 比较两组规范抽样记录的长度和顺序。
// 输入：left 和 right 是规范记录。
// 输出：完全相同时返回 true。
// 副作用：无。
func stringSlicesEqual(left, right []string) bool {
	// 1. 先比较长度，再逐项比较。
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
