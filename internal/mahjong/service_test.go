package mahjong

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServiceSaveUsesDateUpsert 验证同一天页面录入会插入、跳过或覆盖。
func TestServiceSaveUsesDateUpsert(t *testing.T) {
	// 1. 首次保存日期应插入记录。
	service := newTestService(t)
	created, err := service.Save(context.Background(), WriteRequest{PlayedDate: "2026-07-15", ResultAmount: "10.126"}, "admin")
	if err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if created.Status != "inserted" || created.Record.ResultAmount != "10.13" {
		t.Errorf("first Save() = %#v, want inserted 10.13", created)
	}

	// 2. 相同金额再次保存应保持 unchanged。
	unchanged, err := service.Save(context.Background(), WriteRequest{PlayedDate: "2026-07-15", ResultAmount: "10.13"}, "admin")
	if err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if unchanged.Status != "unchanged" {
		t.Errorf("second status = %q, want unchanged", unchanged.Status)
	}

	// 3. 不同金额再次保存应覆盖原记录。
	updated, err := service.Save(context.Background(), WriteRequest{PlayedDate: "2026-07-15", ResultAmount: "-5"}, "admin")
	if err != nil {
		t.Fatalf("third Save() error = %v", err)
	}
	if updated.Status != "updated" || updated.Record.ResultAmount != "-5.00" {
		t.Errorf("third Save() = %#v, want updated -5.00", updated)
	}
}

// TestServiceReportMatchesLegacyCalculations 验证摘要、趋势和周期统计沿用旧项目口径。
func TestServiceReportMatchesLegacyCalculations(t *testing.T) {
	// 1. 写入一胜一负一平的三天记录。
	service := newTestService(t)
	for _, request := range []WriteRequest{
		{PlayedDate: "2026-07-01", ResultAmount: "100"},
		{PlayedDate: "2026-07-02", ResultAmount: "-50"},
		{PlayedDate: "2026-07-03", ResultAmount: "0"},
	} {
		if _, err := service.Save(context.Background(), request, "admin"); err != nil {
			t.Fatalf("Save(%s) error = %v", request.PlayedDate, err)
		}
	}

	// 2. 报告应保留胜率、实际场均、累计趋势和月度聚合。
	report, err := service.Report(context.Background(), 1000, "9")
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if report.Summary.TotalGames != 3 || report.Summary.WinRate != "33.33" {
		t.Errorf("summary games/rate = %d/%s, want 3/33.33", report.Summary.TotalGames, report.Summary.WinRate)
	}
	if report.Summary.TotalResult != "50.00" || report.Summary.AverageResult != "16.67" || report.Summary.AdjustedAverageResult != "25.67" {
		t.Errorf("summary amounts = %#v", report.Summary)
	}
	if len(report.Timeline) != 3 || report.Timeline[2].CumulativeResult != "50.00" {
		t.Errorf("timeline = %#v, want cumulative 50.00", report.Timeline)
	}
	if len(report.Monthly) != 1 || report.Monthly[0].Period != "2026-07" {
		t.Errorf("monthly = %#v, want 2026-07", report.Monthly)
	}
}

// newTestService 创建使用隔离 MySQL 的麻将测试服务。
func newTestService(t *testing.T) *Service {
	// 1. 打开隔离 MySQL schema 并执行完整迁移。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 返回仓储和服务的真实组合。
	return NewService(NewRepository(db))
}
