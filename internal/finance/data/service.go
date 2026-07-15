package data

import (
	"context"
	"fmt"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
)

// DailySource 定义日线同步服务使用的 Tushare HTTP 能力。
type DailySource interface {
	Daily(ctx context.Context, startDate, endDate string) ([]client.DailyRow, error)
}

// SyncOptions 描述日线补数窗口、请求间隔和可测试时钟。
type SyncOptions struct {
	LookbackDays int
	Delay        time.Duration
	Now          func() time.Time
}

// SyncResult 描述一次日线补数的窗口和处理数量。
type SyncResult struct {
	StartDate    string   `json:"start_date"`
	EndDate      string   `json:"end_date"`
	MissingCount int      `json:"missing_count"`
	SyncedCount  int      `json:"synced_count"`
	EmptyCount   int      `json:"empty_count"`
	RowCount     int      `json:"row_count"`
	SyncedDates  []string `json:"synced_dates"`
}

// Service 负责识别缺失开市日、拉取 Tushare 并事务写入 SQLite。
type Service struct {
	repository *Repository
	source     DailySource
	options    SyncOptions
	location   *time.Location
}

// NewService 创建行情同步服务。
// 输入：repository 提供 SQLite，source 提供 Tushare，options 提供窗口、间隔和时钟。
// 输出：返回使用 Asia/Shanghai 日期口径的服务。
// 副作用：无，不查询数据库或外部接口。
func NewService(repository *Repository, source DailySource, options SyncOptions) *Service {
	// 1. 应用同步窗口和时钟默认值。
	if options.LookbackDays <= 0 {
		options.LookbackDays = 60
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return &Service{repository: repository, source: source, options: options, location: location}
}

// UpdateDaily 补充窗口内全部缺失开市日的股票日线。
// 输入：ctx 控制 SQLite 查询、Tushare 请求和请求间隔。
// 输出：返回同步窗口、日期和行数摘要；任一日期失败时返回错误。
// 副作用：调用 Tushare HTTP 并按交易日事务重写 SQLite tushare_daily。
func (s *Service) UpdateDaily(ctx context.Context) (SyncResult, error) {
	// 1. 计算上海日期窗口，停跑超过窗口时从本地最新日期继续追补。
	today := s.options.Now().In(s.location)
	endDate := today.Format("2006-01-02")
	startDate := today.AddDate(0, 0, -s.options.LookbackDays).Format("2006-01-02")
	latestDate, err := s.repository.LatestDailyDate(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("确定日线同步起点: %w", err)
	}
	if latestDate != "" && latestDate < startDate {
		startDate = latestDate
	}
	result := SyncResult{StartDate: startDate, EndDate: endDate, SyncedDates: []string{}}

	// 2. 从交易日历和日线日期索引读取缺失开市日。
	dates, err := s.repository.MissingOpenDates(ctx, startDate, endDate)
	if err != nil {
		return result, fmt.Errorf("读取缺失开市日: %w", err)
	}
	result.MissingCount = len(dates)
	for index, date := range dates {
		rows, err := s.source.Daily(ctx, date, date)
		if err != nil {
			return result, fmt.Errorf("同步 %s 日线: %w", date, err)
		}
		if len(rows) == 0 {
			result.EmptyCount++
			continue
		}

		// 3. 转换 client 模型并通过仓储原子替换当日完整批次。
		dailyRows := make([]Daily, 0, len(rows))
		for _, row := range rows {
			dailyRows = append(dailyRows, Daily{
				TSCode: row.TSCode, TradeDate: row.TradeDate, Open: row.Open, High: row.High,
				Low: row.Low, Close: row.Close, PreClose: row.PreClose, Change: row.Change,
				PctChange: row.PctChange, Volume: row.Volume, Amount: row.Amount,
			})
		}
		if err := s.repository.ReplaceDailyDate(ctx, date, dailyRows); err != nil {
			return result, fmt.Errorf("保存 %s 日线: %w", date, err)
		}
		result.SyncedCount++
		result.RowCount += len(dailyRows)
		result.SyncedDates = append(result.SyncedDates, date)

		// 4. 在相邻外部请求之间等待配置间隔并响应取消。
		if s.options.Delay > 0 && index < len(dates)-1 {
			timer := time.NewTimer(s.options.Delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return result, fmt.Errorf("等待下一次 Tushare 请求: %w", ctx.Err())
			}
		}
	}
	return result, nil
}
