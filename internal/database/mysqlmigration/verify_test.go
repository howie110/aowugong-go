package mysqlmigration

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
)

// TestVerifyTablesChecksCountsRangesAndSamples 验证逐表核验会发现抽样关键字段差异。
// 输入：行数和日期范围相同但中间样本名称不同的源表与目标表。
// 输出：核验报告包含行数、日期范围，并把 samples 标记为不匹配。
// 副作用：在测试临时目录创建并写入两个 SQLite 文件。
func TestVerifyTablesChecksCountsRangesAndSamples(t *testing.T) {
	// 1. 创建结构相同且三行数据近似的源表和目标表。
	ctx := context.Background()
	source := openMigrationTestDB(t, filepath.Join(t.TempDir(), "verify-source.db"))
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "verify-target.db"))
	for _, db := range []*sql.DB{source, target} {
		if _, err := db.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY, day TEXT, name TEXT)"); err != nil {
			t.Fatalf("create sample: %v", err)
		}
	}
	for index, name := range []string{"one", "two", "three"} {
		id := index + 1
		if _, err := source.ExecContext(ctx, "INSERT INTO sample VALUES(?,?,?)", id, "2026-01-0"+strconv.Itoa(id), name); err != nil {
			t.Fatalf("insert source: %v", err)
		}
		targetName := name
		if id == 2 {
			targetName = "changed"
		}
		if _, err := target.ExecContext(ctx, "INSERT INTO sample VALUES(?,?,?)", id, "2026-01-0"+strconv.Itoa(id), targetName); err != nil {
			t.Fatalf("insert target: %v", err)
		}
	}
	plan := TablePlan{
		Name: "sample", SourceColumns: []string{"id", "day", "name"},
		TargetColumns: []string{"id", "day", "name"}, SourceTypes: []string{"integer", "date", "varchar"},
		PrimaryKey: "id", RangeColumns: []string{"day"},
	}

	// 2. 核对行数和日期范围通过，但抽样差异让整表失败。
	report, err := VerifyTables(ctx, source, target, []TablePlan{plan})
	if err != nil {
		t.Fatalf("VerifyTables() error = %v", err)
	}
	if len(report) != 1 || report[0].SourceCount != 3 || report[0].TargetCount != 3 {
		t.Fatalf("report = %+v", report)
	}
	if report[0].Ranges["day"].SourceMin != "2026-01-01" || report[0].Ranges["day"].TargetMax != "2026-01-03" {
		t.Errorf("ranges = %+v", report[0].Ranges)
	}
	if report[0].SamplesMatched || report[0].Passed {
		t.Errorf("verification unexpectedly passed: %+v", report[0])
	}
}

// TestVerifyTablesUsesStableOrderingWithoutPrimaryKey 验证无主键表不受插入顺序影响。
// 输入：内容相同但插入顺序相反且首字段重复的源表和目标表。
// 输出：使用全部字段稳定排序后抽样和整表核验通过。
// 副作用：在测试临时目录创建并写入两个 SQLite 文件。
func TestVerifyTablesUsesStableOrderingWithoutPrimaryKey(t *testing.T) {
	// 1. 创建无主键表并以相反顺序写入相同数据。
	ctx := context.Background()
	source := openMigrationTestDB(t, filepath.Join(t.TempDir(), "no-pk-source.db"))
	target := openMigrationTestDB(t, filepath.Join(t.TempDir(), "no-pk-target.db"))
	for _, db := range []*sql.DB{source, target} {
		if _, err := db.ExecContext(ctx, "CREATE TABLE calendar(exchange TEXT, day TEXT)"); err != nil {
			t.Fatalf("create calendar: %v", err)
		}
	}
	for _, day := range []string{"01", "02", "03", "04"} {
		if _, err := source.ExecContext(ctx, "INSERT INTO calendar VALUES('SSE',?)", day); err != nil {
			t.Fatalf("insert source: %v", err)
		}
	}
	for _, day := range []string{"04", "03", "02", "01"} {
		if _, err := target.ExecContext(ctx, "INSERT INTO calendar VALUES('SSE',?)", day); err != nil {
			t.Fatalf("insert target: %v", err)
		}
	}
	plan := TablePlan{
		Name: "calendar", SourceColumns: []string{"exchange", "day"},
		TargetColumns: []string{"exchange", "day"}, SourceTypes: []string{"text", "text"},
		RangeColumns: []string{"day"},
	}

	// 2. 核对无主键抽样仍通过。
	report, err := VerifyTables(ctx, source, target, []TablePlan{plan})
	if err != nil {
		t.Fatalf("VerifyTables() error = %v", err)
	}
	if len(report) != 1 || !report[0].SamplesMatched || !report[0].Passed {
		t.Errorf("report = %+v", report)
	}
}

// TestCanonicalValuePreservesExactIntegerAndDecimal 验证核验不会用 float64 合并不同精确数值。
// 输入：超过 JavaScript 安全整数的 BIGINT 和不同末位的高精度 DECIMAL。
// 输出：整数保持全部数字，小数去除无意义尾零但保留有效末位差异。
// 副作用：无。
func TestCanonicalValuePreservesExactIntegerAndDecimal(t *testing.T) {
	// 1. BIGINT 必须保持超过 2^53 后的精确末位。
	if got := canonicalValue([]byte("9007199254740993"), "bigint"); got != "9007199254740993" {
		t.Errorf("canonical BIGINT = %q", got)
	}

	// 2. DECIMAL 应统一尾零，同时不能把相邻高精度小数视为相同。
	left := canonicalValue([]byte("1234567890123456.1200"), "decimal")
	right := canonicalValue([]byte("1234567890123456.1201"), "decimal")
	if left != "30864197253086403/25" {
		t.Errorf("canonical DECIMAL = %q", left)
	}
	if left == right {
		t.Fatalf("different decimals collapsed to %q", left)
	}
}
