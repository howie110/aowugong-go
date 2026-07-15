package mysqlmigration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildPlansUsesTargetOrderAndSourceTypes 验证迁移计划按 SQLite 字段顺序映射 MySQL。
// 输入：字段顺序不同但名称一致的源盘点和目标表。
// 输出：计划使用目标顺序、源类型和单主键，并保留有效范围字段。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestBuildPlansUsesTargetOrderAndSourceTypes(t *testing.T) {
	// 1. 创建目标表和字段顺序不同的源盘点。
	ctx := context.Background()
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "plan.db"))
	if _, err := target.ExecContext(ctx, "CREATE TABLE sample(name TEXT NOT NULL, id INTEGER PRIMARY KEY, day TEXT)"); err != nil {
		t.Fatalf("create sample: %v", err)
	}
	inventory := Inventory{Tables: []TableInventory{{
		Name: "sample",
		Columns: []ColumnInventory{
			{Name: "id", DataType: "bigint", PrimaryKey: true},
			{Name: "day", DataType: "date"},
			{Name: "name", DataType: "varchar"},
		},
	}}}

	// 2. 构建并核对一一映射、类型、主键和范围。
	plans, err := BuildPlans(ctx, target, inventory, []TableSpec{{Name: "sample", RangeColumns: []string{"day"}}}, nil)
	if err != nil {
		t.Fatalf("BuildPlans() error = %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("plan count = %d, want 1", len(plans))
	}
	plan := plans[0]
	if plan.PrimaryKey != "id" || plan.TargetColumns[0] != "name" || plan.SourceColumns[0] != "name" {
		t.Errorf("plan = %+v", plan)
	}
	if plan.SourceTypes[2] != "date" || len(plan.RangeColumns) != 1 || plan.RangeColumns[0] != "day" {
		t.Errorf("types/ranges = %#v / %#v", plan.SourceTypes, plan.RangeColumns)
	}
}

// TestBuildPlansRejectsUnmappedSourceColumns 验证源字段不会在未审计时被静默丢弃。
// 输入：比目标多一个历史字段的源表盘点。
// 输出：未显式忽略时返回错误，显式忽略后生成计划。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestBuildPlansRejectsUnmappedSourceColumns(t *testing.T) {
	// 1. 创建目标表和包含额外字段的源盘点。
	ctx := context.Background()
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "strict-plan.db"))
	if _, err := target.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create sample: %v", err)
	}
	inventory := Inventory{Tables: []TableInventory{{
		Name: "sample",
		Columns: []ColumnInventory{
			{Name: "id", DataType: "bigint", PrimaryKey: true},
			{Name: "name", DataType: "varchar"},
			{Name: "legacy_note", DataType: "varchar"},
		},
	}}}

	// 2. 未声明忽略时必须报出具体源字段。
	_, err := BuildPlans(ctx, target, inventory, []TableSpec{{Name: "sample"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "legacy_note") {
		t.Fatalf("BuildPlans() error = %v, want unmapped legacy_note", err)
	}

	// 3. 显式记录忽略字段后才允许生成迁移计划。
	plans, err := BuildPlans(ctx, target, inventory, []TableSpec{{
		Name: "sample", IgnoredSourceColumns: []string{"legacy_note"},
	}}, nil)
	if err != nil {
		t.Fatalf("BuildPlans() with ignored source error = %v", err)
	}
	if len(plans) != 1 || len(plans[0].SourceColumns) != 2 {
		t.Fatalf("plans = %#v", plans)
	}
}

// TestBuildPlansRejectsMissingRangeColumn 验证配置的关键核验字段不能静默失效。
// 输入：规格声明目标表不存在的范围字段。
// 输出：返回包含字段名的计划错误。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestBuildPlansRejectsMissingRangeColumn(t *testing.T) {
	// 1. 创建最小同构源表和目标表。
	ctx := context.Background()
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "range-plan.db"))
	if _, err := target.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create sample: %v", err)
	}
	inventory := Inventory{Tables: []TableInventory{{
		Name: "sample", Columns: []ColumnInventory{{Name: "id", DataType: "bigint", PrimaryKey: true}},
	}}}

	// 2. 缺失范围字段必须阻止生成弱化核验计划。
	_, err := BuildPlans(ctx, target, inventory, []TableSpec{{Name: "sample", RangeColumns: []string{"missing"}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("BuildPlans() error = %v, want missing range column", err)
	}
}
