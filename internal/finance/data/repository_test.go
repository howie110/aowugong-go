package data

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestRepositoryReplacesDailyDateAndQueriesByRange 验证日线按交易日原子替换和范围查询。
// 输入：同一交易日的两批日线以及一个代码和日期范围。
// 输出：旧批次被完整替换，查询仅返回目标代码范围内的数据。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestRepositoryReplacesDailyDateAndQueriesByRange(t *testing.T) {
	// 1. 创建完整迁移数据库和日线仓储。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)

	// 2. 首次写入两个代码，再以同日单代码批次原子替换。
	first := []Daily{
		{TSCode: "000001.SZ", TradeDate: "2026-01-02", Open: 10, High: 12, Low: 9, Close: 11, PreClose: 9.5},
		{TSCode: "600000.SH", TradeDate: "2026-01-02", Open: 20, High: 21, Low: 19, Close: 20.5, PreClose: 20},
	}
	if err := repository.ReplaceDailyDate(ctx, "2026-01-02", first); err != nil {
		t.Fatalf("first ReplaceDailyDate() error = %v", err)
	}
	replacement := []Daily{
		{TSCode: "000001.SZ", TradeDate: "2026-01-02", Open: 11, High: 13, Low: 10, Close: 12, PreClose: 11},
	}
	if err := repository.ReplaceDailyDate(ctx, "2026-01-02", replacement); err != nil {
		t.Fatalf("second ReplaceDailyDate() error = %v", err)
	}

	// 3. 核对范围查询结果和同日替换后的全表行数。
	rows, err := repository.DailyByCode(ctx, "000001.SZ", "2026-01-01", "2026-01-03")
	if err != nil {
		t.Fatalf("DailyByCode() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Close != 12 {
		t.Errorf("rows = %#v", rows)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tushare_daily").Scan(&count); err != nil {
		t.Fatalf("count tushare_daily: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}
}

// TestRepositoryListsMissingOpenDates 验证同步任务只选择缺少日线的开市日期。
// 输入：三天交易日历，其中一天休市，一天已有日线。
// 输出：只返回仍缺失的开市日。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestRepositoryListsMissingOpenDates(t *testing.T) {
	// 1. 创建数据库并写入交易日历与一个已有交易日。
	ctx := context.Background()
	db := testdatabase.Open(t)
	_, err := db.ExecContext(ctx, `INSERT INTO tushare_trade_cal(exchange, cal_date, is_open, pretrade_date) VALUES
		('SSE','2026-01-01',0,'2025-12-31'),('SSE','2026-01-02',1,'2025-12-31'),('SSE','2026-01-05',1,'2026-01-02')`)
	if err != nil {
		t.Fatalf("insert calendar: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.ReplaceDailyDate(ctx, "2026-01-02", []Daily{{
		TSCode: "000001.SZ", TradeDate: "2026-01-02", Open: 10, High: 10, Low: 10, Close: 10, PreClose: 10,
	}}); err != nil {
		t.Fatalf("ReplaceDailyDate() error = %v", err)
	}

	// 2. 查询窗口内缺失开市日并核对唯一日期。
	dates, err := repository.MissingOpenDates(ctx, "2026-01-01", "2026-01-05")
	if err != nil {
		t.Fatalf("MissingOpenDates() error = %v", err)
	}
	if len(dates) != 1 || dates[0] != "2026-01-05" {
		t.Errorf("dates = %#v, want [2026-01-05]", dates)
	}
}
