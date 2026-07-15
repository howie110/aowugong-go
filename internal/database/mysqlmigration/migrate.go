package mysqlmigration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ForeignKeyViolation 描述 SQLite 外键完整性检查的一条异常。
type ForeignKeyViolation struct {
	Table      string `json:"table"`
	RowID      *int64 `json:"row_id,omitempty"`
	Parent     string `json:"parent"`
	ForeignKey int    `json:"foreign_key"`
}

// TableMigration 描述一张表的复制行数、耗时和核验结果。
type TableMigration struct {
	Name         string            `json:"name"`
	CopiedRows   int64             `json:"copied_rows"`
	DurationMS   int64             `json:"duration_ms"`
	Verification TableVerification `json:"verification"`
}

// MigrationReport 描述一次 MySQL 到 SQLite 复制与核验的最终结果。
type MigrationReport struct {
	StartedAt            string                `json:"started_at"`
	FinishedAt           string                `json:"finished_at"`
	DurationMS           int64                 `json:"duration_ms"`
	Tables               []TableMigration      `json:"tables"`
	ForeignKeyViolations []ForeignKeyViolation `json:"foreign_key_violations"`
	Passed               bool                  `json:"passed"`
}

// Migrate 清空目标有效表后批量复制、检查外键并执行逐表核验。
// 输入：ctx 控制执行，source 只读 MySQL，target 是 SQLite，plans 是计划，batchSize 是批次。
// 输出：返回完整报告；复制、外键或核验失败时同时返回报告和错误。
// 副作用：只读 MySQL，删除并重写目标 SQLite 计划内表。
func Migrate(ctx context.Context, source SourceReader, target *sql.DB, plans []TablePlan, batchSize int, progress ProgressFunc) (MigrationReport, error) {
	// 1. 初始化报告并暂时关闭 SQLite 外键触发，便于重复清空父子表。
	startedAt := time.Now()
	report := MigrationReport{
		StartedAt: startedAt.Format(time.RFC3339), Tables: make([]TableMigration, 0, len(plans)),
		ForeignKeyViolations: []ForeignKeyViolation{},
	}
	if source == nil || target == nil || len(plans) == 0 {
		return finishReport(report, startedAt, false), fmt.Errorf("源库、目标库和迁移计划不能为空")
	}
	if _, err := target.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return finishReport(report, startedAt, false), fmt.Errorf("关闭 SQLite 外键检查: %w", err)
	}
	foreignKeysEnabled := false
	defer func() {
		if !foreignKeysEnabled {
			_, _ = target.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
		}
	}()

	// 2. 按审计顺序逐表执行可重复批量复制。
	for _, plan := range plans {
		tableStartedAt := time.Now()
		copied, err := copyTable(ctx, source, target, plan, batchSize, progress)
		tableReport := TableMigration{
			Name: plan.Name, CopiedRows: copied, DurationMS: time.Since(tableStartedAt).Milliseconds(),
		}
		report.Tables = append(report.Tables, tableReport)
		if err != nil {
			return finishReport(report, startedAt, false), fmt.Errorf("迁移表 %s: %w", plan.Name, err)
		}
	}

	// 3. 重新启用外键并检查所有引用完整性。
	if _, err := target.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return finishReport(report, startedAt, false), fmt.Errorf("启用 SQLite 外键检查: %w", err)
	}
	foreignKeysEnabled = true
	violations, err := checkForeignKeys(ctx, target)
	if err != nil {
		return finishReport(report, startedAt, false), err
	}
	report.ForeignKeyViolations = violations
	if len(violations) > 0 {
		return finishReport(report, startedAt, false), fmt.Errorf("SQLite 存在 %d 条外键异常", len(violations))
	}

	// 4. 比较行数、关键范围和首尾抽样记录，并写回各表报告。
	verifications, err := VerifyTables(ctx, source, target, plans)
	if err != nil {
		return finishReport(report, startedAt, false), err
	}
	verificationMap := make(map[string]TableVerification, len(verifications))
	allPassed := true
	for _, verification := range verifications {
		verificationMap[verification.Name] = verification
		if !verification.Passed {
			allPassed = false
		}
	}
	for index := range report.Tables {
		report.Tables[index].Verification = verificationMap[report.Tables[index].Name]
	}
	report = finishReport(report, startedAt, allPassed)
	if !allPassed {
		return report, fmt.Errorf("MySQL 到 SQLite 逐表核验未全部通过")
	}
	return report, nil
}

// VerifyOnly 在不复制数据时执行外键和逐表数据核验。
// 输入：ctx 控制查询，source/target 是数据库，plans 是迁移计划。
// 输出：返回核验结果和外键异常；查询失败时返回错误。
// 副作用：只读 MySQL 和 SQLite。
func VerifyOnly(ctx context.Context, source SourceReader, target *sql.DB, plans []TablePlan) ([]TableVerification, []ForeignKeyViolation, error) {
	// 1. 先检查目标外键，再比较全部计划表。
	violations, err := checkForeignKeys(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	verifications, err := VerifyTables(ctx, source, target, plans)
	if err != nil {
		return nil, violations, err
	}
	return verifications, violations, nil
}

// checkForeignKeys 执行 SQLite 全库外键完整性检查。
// 输入：ctx 控制查询，target 是 SQLite。
// 输出：返回全部异常，完整时返回空切片。
// 副作用：只读 SQLite。
func checkForeignKeys(ctx context.Context, target *sql.DB) ([]ForeignKeyViolation, error) {
	// 1. 扫描 PRAGMA foreign_key_check 的标准四列结果。
	rows, err := target.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, fmt.Errorf("检查 SQLite 外键: %w", err)
	}
	defer rows.Close()
	result := make([]ForeignKeyViolation, 0)
	for rows.Next() {
		var item ForeignKeyViolation
		var rowID sql.NullInt64
		if err := rows.Scan(&item.Table, &rowID, &item.Parent, &item.ForeignKey); err != nil {
			return nil, fmt.Errorf("扫描 SQLite 外键异常: %w", err)
		}
		if rowID.Valid {
			item.RowID = &rowID.Int64
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 SQLite 外键异常: %w", err)
	}
	return result, nil
}

// finishReport 补充结束时间、耗时和最终通过状态。
// 输入：report 是当前报告，startedAt 是开始时间，passed 是最终状态。
// 输出：返回补充完成的报告。
// 副作用：读取系统时钟。
func finishReport(report MigrationReport, startedAt time.Time, passed bool) MigrationReport {
	// 1. 使用同一个结束时间计算报告字段。
	finishedAt := time.Now()
	report.FinishedAt = finishedAt.Format(time.RFC3339)
	report.DurationMS = finishedAt.Sub(startedAt).Milliseconds()
	report.Passed = passed
	return report
}
