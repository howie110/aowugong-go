package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Repository 负责 finance 行情表的 SQLite 查询与事务写入。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建行情数据仓储。
// 输入：db 是已完成迁移的 SQLite 连接。
// 输出：返回可并发复用的仓储。
// 副作用：无，不执行 SQL。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存 SQLite 依赖供受限查询和事务写入复用。
	return &Repository{db: db}
}

// ReplaceDailyDate 在单个事务中替换一个交易日的全部股票日线。
// 输入：ctx 控制事务，tradeDate 是目标日期，rows 是该日完整上游数据。
// 输出：成功返回 nil；日期、数据或 SQL 无效时返回错误。
// 副作用：删除并重写 SQLite tushare_daily 指定日期的数据。
func (r *Repository) ReplaceDailyDate(ctx context.Context, tradeDate string, rows []Daily) error {
	// 1. 在删除旧数据前校验完整批次，防止空响应清空已有行情。
	tradeDate = strings.TrimSpace(tradeDate)
	if tradeDate == "" {
		return fmt.Errorf("交易日期不能为空")
	}
	if len(rows) == 0 {
		return fmt.Errorf("%s 日线批次不能为空", tradeDate)
	}
	for index, row := range rows {
		if strings.TrimSpace(row.TSCode) == "" || row.TradeDate != tradeDate {
			return fmt.Errorf("第 %d 条日线代码为空或日期不属于 %s", index+1, tradeDate)
		}
	}

	// 2. 开启事务并删除目标交易日旧批次。
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始替换 %s 日线事务: %w", tradeDate, err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, "DELETE FROM tushare_daily WHERE trade_date = ?", tradeDate); err != nil {
		return fmt.Errorf("删除 %s 旧日线: %w", tradeDate, err)
	}

	// 3. 复用预编译语句写入完整新批次。
	statement, err := transaction.PrepareContext(ctx, `INSERT INTO tushare_daily(
		ts_code, trade_date, open, high, low, close, pre_close, change, pct_chg, vol, amount, create_date, update_date
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("准备写入 %s 日线: %w", tradeDate, err)
	}
	defer statement.Close()
	now := time.Now().Format(time.RFC3339)
	for index, row := range rows {
		if _, err := statement.ExecContext(ctx,
			row.TSCode, row.TradeDate, row.Open, row.High, row.Low, row.Close,
			row.PreClose, row.Change, row.PctChange, row.Volume, row.Amount, now, now,
		); err != nil {
			return fmt.Errorf("写入 %s 第 %d 条日线: %w", tradeDate, index+1, err)
		}
	}

	// 4. 仅在完整批次写入成功后提交事务。
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("提交 %s 日线事务: %w", tradeDate, err)
	}
	return nil
}

// DailyByCode 按股票代码和闭区间日期读取升序日线。
// 输入：ctx 控制查询，tsCode 是代码，startDate 和 endDate 是范围。
// 输出：返回范围内日线；参数或查询失败时返回错误。
// 副作用：只读 SQLite，禁止无条件全表查询。
func (r *Repository) DailyByCode(ctx context.Context, tsCode, startDate, endDate string) ([]Daily, error) {
	// 1. 校验大表查询必须包含代码和有效日期范围。
	tsCode = strings.TrimSpace(tsCode)
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	if tsCode == "" || startDate == "" || endDate == "" || startDate > endDate {
		return nil, fmt.Errorf("股票代码和有效起止日期不能为空")
	}

	// 2. 使用复合索引条件查询并按交易日升序扫描。
	rows, err := r.db.QueryContext(ctx, `SELECT id, ts_code, trade_date, open, high, low, close, pre_close,
		change, pct_chg, vol, amount, COALESCE(create_date,''), COALESCE(update_date,'')
		FROM tushare_daily WHERE ts_code = ? AND trade_date >= ? AND trade_date <= ? ORDER BY trade_date`,
		tsCode, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("查询 %s 日线: %w", tsCode, err)
	}
	defer rows.Close()
	result := make([]Daily, 0)
	for rows.Next() {
		var row Daily
		if err := rows.Scan(&row.ID, &row.TSCode, &row.TradeDate, &row.Open, &row.High, &row.Low,
			&row.Close, &row.PreClose, &row.Change, &row.PctChange, &row.Volume, &row.Amount,
			&row.CreateDate, &row.UpdateDate); err != nil {
			return nil, fmt.Errorf("扫描 %s 日线: %w", tsCode, err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 %s 日线: %w", tsCode, err)
	}
	return result, nil
}

// MissingOpenDates 查询日期范围内尚无股票日线的开市日。
// 输入：ctx 控制查询，startDate 和 endDate 是闭区间。
// 输出：返回升序缺失日期；参数或查询失败时返回错误。
// 副作用：只读 SQLite 的交易日历和日线日期索引。
func (r *Repository) MissingOpenDates(ctx context.Context, startDate, endDate string) ([]string, error) {
	// 1. 校验同步窗口，禁止无边界扫描交易日历或日线大表。
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	if startDate == "" || endDate == "" || startDate > endDate {
		return nil, fmt.Errorf("有效起止日期不能为空")
	}

	// 2. 通过索引相关子查询筛选没有任何日线的开市日。
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT cal.cal_date
		FROM tushare_trade_cal AS cal
		WHERE cal.is_open = 1 AND cal.cal_date >= ? AND cal.cal_date <= ?
		AND NOT EXISTS (SELECT 1 FROM tushare_daily AS daily WHERE daily.trade_date = cal.cal_date)
		ORDER BY cal.cal_date`, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("查询缺失交易日: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("扫描缺失交易日: %w", err)
		}
		result = append(result, date)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历缺失交易日: %w", err)
	}
	return result, nil
}

// LatestDailyDate 读取本地股票日线的最新交易日。
// 输入：ctx 控制查询。
// 输出：有数据时返回日期，无数据时返回空字符串。
// 副作用：只读 SQLite 的交易日索引。
func (r *Repository) LatestDailyDate(ctx context.Context) (string, error) {
	// 1. 使用索引聚合查询最新日期并兼容空表。
	var value sql.NullString
	if err := r.db.QueryRowContext(ctx, "SELECT MAX(trade_date) FROM tushare_daily").Scan(&value); err != nil {
		return "", fmt.Errorf("读取最新日线日期: %w", err)
	}
	return value.String, nil
}
