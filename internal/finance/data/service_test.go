package data

import (
	"context"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type fakeDailySource struct {
	dates []string
}

// Daily 返回单个测试交易日的一条行情。
// 输入：上下文和起止日期。
// 输出：返回日期匹配的测试行情。
// 副作用：记录被请求的开始日期。
func (s *fakeDailySource) Daily(_ context.Context, startDate, _ string) ([]client.DailyRow, error) {
	// 1. 记录日期并构造一条完整日线。
	s.dates = append(s.dates, startDate)
	return []client.DailyRow{{
		TSCode: "000001.SZ", TradeDate: startDate, Open: 10, High: 11,
		Low: 9, Close: 10.5, PreClose: 10, Volume: 1000, Amount: 10500,
	}}, nil
}

// TestServiceUpdatesOnlyMissingOpenDates 验证日线同步只拉取缺失开市日并写入 SQLite。
// 输入：一个已有开市日、一个缺失开市日、一个休市日和模拟 Tushare。
// 输出：只请求并写入缺失开市日，返回日期与行数摘要。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestServiceUpdatesOnlyMissingOpenDates(t *testing.T) {
	// 1. 创建数据库、交易日历和一个已有交易日。
	ctx := context.Background()
	db := testdatabase.Open(t)
	_, err := db.ExecContext(ctx, `INSERT INTO tushare_trade_cal(exchange, cal_date, is_open) VALUES
		('SSE','2026-07-13',1),('SSE','2026-07-14',1),('SSE','2026-07-15',0)`)
	if err != nil {
		t.Fatalf("insert calendar: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.ReplaceDailyDate(ctx, "2026-07-13", []Daily{{
		TSCode: "000001.SZ", TradeDate: "2026-07-13", Open: 10, High: 10, Low: 10, Close: 10, PreClose: 10,
	}}); err != nil {
		t.Fatalf("seed daily: %v", err)
	}
	source := &fakeDailySource{}
	service := NewService(repository, source, SyncOptions{
		LookbackDays: 60,
		Now:          func() time.Time { return time.Date(2026, 7, 15, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)) },
	})

	// 2. 执行同步并核对仅请求缺失开市日。
	result, err := service.UpdateDaily(context.Background())
	if err != nil {
		t.Fatalf("UpdateDaily() error = %v", err)
	}
	if len(source.dates) != 1 || source.dates[0] != "2026-07-14" {
		t.Errorf("requested dates = %#v", source.dates)
	}
	if result.MissingCount != 1 || result.SyncedCount != 1 || result.RowCount != 1 {
		t.Errorf("result = %+v", result)
	}

	// 3. 核对新日线已持久化且再次执行不会重复请求。
	rows, err := repository.DailyByCode(ctx, "000001.SZ", "2026-07-14", "2026-07-14")
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored rows = %#v error = %v", rows, err)
	}
	second, err := service.UpdateDaily(context.Background())
	if err != nil {
		t.Fatalf("second UpdateDaily() error = %v", err)
	}
	if second.MissingCount != 0 || len(source.dates) != 1 {
		t.Errorf("second result = %+v dates = %#v", second, source.dates)
	}
}
