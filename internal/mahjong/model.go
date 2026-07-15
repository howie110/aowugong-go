// Package mahjong 提供单日麻将战绩录入、Excel 导入和统计报告。
package mahjong

import "errors"

var ErrInvalidInput = errors.New("麻将战绩参数无效")

// WriteRequest 描述页面录入的日期和当日输赢。
type WriteRequest struct {
	PlayedDate   string `json:"played_date"`
	ResultAmount string `json:"result_amount"`
}

// Record 描述已持久化的一条麻将战绩。
type Record struct {
	ID             int64   `json:"id"`
	PlayedDate     string  `json:"played_date"`
	ResultAmount   string  `json:"result_amount"`
	SourceFilename *string `json:"source_filename"`
	CreatedBy      *string `json:"created_by"`
	CreatedAt      *string `json:"created_at"`
	UpdatedAt      *string `json:"updated_at"`
}

// WriteResponse 描述页面录入后的插入、更新或不变状态。
type WriteResponse struct {
	Status string `json:"status"`
	Record Record `json:"record"`
}

// ImportResponse 描述 Excel 导入的解析和写入统计。
type ImportResponse struct {
	Filename      string  `json:"filename"`
	ParsedCount   int     `json:"parsed_count"`
	InsertedCount int     `json:"inserted_count"`
	UpdatedCount  int     `json:"updated_count"`
	SkippedCount  int     `json:"skipped_count"`
	LatestRecord  *Record `json:"latest_record"`
}

// ExtremeDay 描述最大单日输赢记录。
type ExtremeDay struct {
	PlayedDate   *string `json:"played_date"`
	ResultAmount string  `json:"result_amount"`
}

// Summary 描述麻将战绩汇总指标。
type Summary struct {
	TotalGames            int        `json:"total_games"`
	WinGames              int        `json:"win_games"`
	LossGames             int        `json:"loss_games"`
	DrawGames             int        `json:"draw_games"`
	WinRate               string     `json:"win_rate"`
	TotalResult           string     `json:"total_result"`
	AverageResult         string     `json:"average_result"`
	TableFee              string     `json:"table_fee"`
	AdjustedAverageResult string     `json:"adjusted_average_result"`
	FirstDate             *string    `json:"first_date"`
	LatestDate            *string    `json:"latest_date"`
	SpanDays              int        `json:"span_days"`
	SpanYears             string     `json:"span_years"`
	BestDay               ExtremeDay `json:"best_day"`
	WorstDay              ExtremeDay `json:"worst_day"`
	CurrentStreakType     string     `json:"current_streak_type"`
	CurrentStreakCount    int        `json:"current_streak_count"`
	LongestWinStreak      int        `json:"longest_win_streak"`
	LongestLossStreak     int        `json:"longest_loss_streak"`
}

// TimelinePoint 描述累计输赢趋势中的一个点。
type TimelinePoint struct {
	Sequence         int    `json:"sequence"`
	PlayedDate       string `json:"played_date"`
	ResultAmount     string `json:"result_amount"`
	CumulativeResult string `json:"cumulative_result"`
	RunningAverage   string `json:"running_average"`
}

// PeriodSummary 描述按月或按年的聚合结果。
type PeriodSummary struct {
	Period        string `json:"period"`
	GameCount     int    `json:"game_count"`
	WinCount      int    `json:"win_count"`
	LossCount     int    `json:"loss_count"`
	WinRate       string `json:"win_rate"`
	TotalResult   string `json:"total_result"`
	AverageResult string `json:"average_result"`
}

// Gap 描述两次打牌之间的日期间隔。
type Gap struct {
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Days      int     `json:"days"`
}

// WeekdayFrequency 描述某星期的打牌频率。
type WeekdayFrequency struct {
	Weekday       int    `json:"weekday"`
	Label         string `json:"label"`
	GameCount     int    `json:"game_count"`
	WeightPercent string `json:"weight_percent"`
}

// FrequencyStats 描述麻将打牌频率统计。
type FrequencyStats struct {
	ActiveMonths            int                `json:"active_months"`
	AverageGamesPerMonth    string             `json:"average_games_per_month"`
	AverageDaysBetweenGames string             `json:"average_days_between_games"`
	Recent90DayGames        int                `json:"recent_90_day_games"`
	Recent365DayGames       int                `json:"recent_365_day_games"`
	MostActiveMonth         *string            `json:"most_active_month"`
	MostActiveMonthGames    int                `json:"most_active_month_games"`
	FavoriteWeekday         *string            `json:"favorite_weekday"`
	FavoriteWeekdayGames    int                `json:"favorite_weekday_games"`
	LongestGap              Gap                `json:"longest_gap"`
	WeekdayDistribution     []WeekdayFrequency `json:"weekday_distribution"`
}

// Report 描述麻将页面完整报告。
type Report struct {
	Summary       Summary         `json:"summary"`
	Frequency     FrequencyStats  `json:"frequency"`
	Timeline      []TimelinePoint `json:"timeline"`
	Monthly       []PeriodSummary `json:"monthly"`
	Yearly        []PeriodSummary `json:"yearly"`
	RecentRecords []Record        `json:"recent_records"`
	RecordCount   int             `json:"record_count"`
}

type storedRecord struct {
	Record
	amountCents int64
}

type parsedRecord struct {
	playedDate  string
	amountCents int64
}

type writeStats struct {
	insertedCount int
	updatedCount  int
	skippedCount  int
	latestRecord  *storedRecord
}
