package mysqlmigration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateCopiesVerifiesAndReportsTables 验证迁移编排执行复制、外键检查和逐表核验。
// 输入：三行源表、含旧行的目标表和单表计划。
// 输出：报告复制三行且核验通过，目标只保留源数据。
// 副作用：在测试临时目录创建并写入两个 SQLite 文件。
func TestMigrateCopiesVerifiesAndReportsTables(t *testing.T) {
	// 1. 创建源和目标表并写入待迁移数据。
	ctx := context.Background()
	source := openMigrationTestDB(t, filepath.Join(t.TempDir(), "migrate-source.db"))
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "migrate-target.db"))
	for _, db := range []*sql.DB{source, target} {
		if _, err := db.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY, value TEXT)"); err != nil {
			t.Fatalf("create sample: %v", err)
		}
	}
	for index := 1; index <= 3; index++ {
		if _, err := source.ExecContext(ctx, "INSERT INTO sample VALUES(?,?)", index, "value"); err != nil {
			t.Fatalf("insert source: %v", err)
		}
	}
	if _, err := target.ExecContext(ctx, "INSERT INTO sample VALUES(99,'old')"); err != nil {
		t.Fatalf("insert target: %v", err)
	}
	plan := TablePlan{
		Name: "sample", SourceColumns: []string{"id", "value"}, TargetColumns: []string{"id", "value"},
		SourceTypes: []string{"integer", "varchar"}, PrimaryKey: "id", RangeColumns: []string{"id"},
	}

	// 2. 执行迁移并核对报告和目标数据。
	report, err := Migrate(ctx, source, target, []TablePlan{plan}, 2, nil)
	if err != nil {
		t.Fatalf("Migrate() error = %v report = %+v", err, report)
	}
	if !report.Passed || len(report.Tables) != 1 || report.Tables[0].CopiedRows != 3 || !report.Tables[0].Verification.Passed {
		t.Errorf("report = %+v", report)
	}
	var count int
	if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM sample").Scan(&count); err != nil || count != 3 {
		t.Errorf("target count = %d error = %v", count, err)
	}
}
