package mahjong

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/howiedata/aowugong-go/internal/money"
)

// Repository 负责麻将战绩的 MySQL 持久化。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建麻将战绩仓储。
// 输入：db 是已完成迁移的 MySQL 连接池。
// 输出：返回麻将仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// Upsert 按日期批量新增、跳过或覆盖麻将战绩。
// 输入：ctx 是调用上下文，records 是已校验记录，sourceFilename 和 createdBy 是来源信息。
// 输出：返回写入统计和最后处理记录。
// 副作用：在单个事务中写入 MySQL。
func (r *Repository) Upsert(ctx context.Context, records []parsedRecord, sourceFilename, createdBy string) (writeStats, error) {
	// 1. 开启事务并逐日期判断现有金额。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return writeStats{}, fmt.Errorf("开始麻将写入事务: %w", err)
	}
	defer tx.Rollback()
	stats := writeStats{}
	var latestID int64
	for _, record := range records {
		var existingID int64
		var existingAmount string
		err := tx.QueryRowContext(ctx, `
			SELECT id, result_amount
			FROM mahjong_game_record
			WHERE played_date = ?
		`, record.playedDate).Scan(&existingID, &existingAmount)
		if errors.Is(err, sql.ErrNoRows) {
			result, err := tx.ExecContext(ctx, `
				INSERT INTO mahjong_game_record (
					played_date, result_amount, source_filename, created_by
				) VALUES (?, ?, ?, ?)
			`, record.playedDate, money.FormatCents(record.amountCents, false), nullText(sourceFilename), nullText(createdBy))
			if err != nil {
				return writeStats{}, fmt.Errorf("新增麻将战绩 %s: %w", record.playedDate, err)
			}
			latestID, err = result.LastInsertId()
			if err != nil {
				return writeStats{}, fmt.Errorf("读取麻将战绩主键: %w", err)
			}
			stats.insertedCount++
			continue
		}
		if err != nil {
			return writeStats{}, fmt.Errorf("查询麻将战绩 %s: %w", record.playedDate, err)
		}

		// 2. 金额未变化时跳过，否则覆盖金额和来源信息。
		existingCents, err := money.ParseCents(existingAmount)
		if err != nil {
			return writeStats{}, fmt.Errorf("解析麻将战绩 %s 金额: %w", record.playedDate, err)
		}
		latestID = existingID
		if existingCents == record.amountCents {
			stats.skippedCount++
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE mahjong_game_record
			SET result_amount = ?, source_filename = ?, created_by = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, money.FormatCents(record.amountCents, false), nullText(sourceFilename), nullText(createdBy), existingID); err != nil {
			return writeStats{}, fmt.Errorf("更新麻将战绩 %s: %w", record.playedDate, err)
		}
		stats.updatedCount++
	}

	// 3. 提交事务后读取最后处理记录。
	if err := tx.Commit(); err != nil {
		return writeStats{}, fmt.Errorf("提交麻将写入事务: %w", err)
	}
	if latestID != 0 {
		latest, err := r.Get(ctx, latestID)
		if err != nil {
			return writeStats{}, err
		}
		stats.latestRecord = &latest
	}
	return stats, nil
}

// Get 按主键读取单条麻将战绩。
// 输入：ctx 是调用上下文，recordID 是主键。
// 输出：返回内部记录。
// 副作用：读取 MySQL。
func (r *Repository) Get(ctx context.Context, recordID int64) (storedRecord, error) {
	// 1. 查询并复用统一行扫描。
	row := r.db.QueryRowContext(ctx, `
		SELECT id, played_date, result_amount, source_filename, created_by, created_at, updated_at
		FROM mahjong_game_record
		WHERE id = ?
	`, recordID)
	record, err := scanRecord(row)
	if err != nil {
		return storedRecord{}, fmt.Errorf("查询麻将战绩: %w", err)
	}
	return record, nil
}

// ListRecentWindow 读取最近 limit 条记录并按日期正序返回。
// 输入：ctx 是调用上下文，limit 是 1 到 5000 的查询上限。
// 输出：返回日期正序记录。
// 副作用：读取 MySQL。
func (r *Repository) ListRecentWindow(ctx context.Context, limit int) ([]storedRecord, error) {
	// 1. 在 SQL 内先限制最近记录，再恢复日期正序。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, played_date, result_amount, source_filename, created_by, created_at, updated_at
		FROM (
			SELECT id, played_date, result_amount, source_filename, created_by, created_at, updated_at
			FROM mahjong_game_record
			ORDER BY played_date DESC
			LIMIT ?
		) recent
		ORDER BY played_date
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询麻将战绩窗口: %w", err)
	}
	defer rows.Close()
	return scanRecords(rows)
}

// ListRecent 读取最近 limit 条记录并按日期倒序返回。
// 输入：ctx 是调用上下文，limit 是 1 到 200 的查询上限。
// 输出：返回日期倒序记录。
// 副作用：读取 MySQL。
func (r *Repository) ListRecent(ctx context.Context, limit int) ([]storedRecord, error) {
	// 1. 使用日期索引直接限制最近记录。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, played_date, result_amount, source_filename, created_by, created_at, updated_at
		FROM mahjong_game_record
		ORDER BY played_date DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近麻将战绩: %w", err)
	}
	defer rows.Close()
	return scanRecords(rows)
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanRecord 扫描并规范化一条麻将数据库记录。
// 输入：scanner 是 QueryRow 或 Rows 的当前行。
// 输出：返回内部记录。
// 副作用：读取数据库游标。
func scanRecord(scanner rowScanner) (storedRecord, error) {
	// 1. 扫描可空来源字段和金额文本。
	var record storedRecord
	var amount string
	var sourceFilename, createdBy, createdAt, updatedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.PlayedDate,
		&amount,
		&sourceFilename,
		&createdBy,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return storedRecord{}, err
	}

	// 2. 解析金额分并构建 API 记录字段。
	amountCents, err := money.ParseCents(amount)
	if err != nil {
		return storedRecord{}, err
	}
	record.amountCents = amountCents
	record.ResultAmount = money.FormatCents(amountCents, false)
	record.SourceFilename = pointerText(sourceFilename)
	record.CreatedBy = pointerText(createdBy)
	record.CreatedAt = pointerText(createdAt)
	record.UpdatedAt = pointerText(updatedAt)
	return record, nil
}

// scanRecords 扫描全部麻将查询结果。
// 输入：rows 是数据库游标。
// 输出：返回内部记录列表。
// 副作用：读取数据库游标。
func scanRecords(rows *sql.Rows) ([]storedRecord, error) {
	// 1. 逐行复用统一扫描逻辑。
	records := make([]storedRecord, 0)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描麻将战绩: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历麻将战绩: %w", err)
	}
	return records, nil
}

// pointerText 把有效数据库文本转换为字符串指针。
// 输入：value 是可空字符串。
// 输出：有效时返回指针，否则返回 nil。
// 副作用：无。
func pointerText(value sql.NullString) *string {
	// 1. 保留数据库 NULL 语义。
	if !value.Valid {
		return nil
	}
	return &value.String
}

// nullText 把空字符串转换为 SQL NULL。
// 输入：value 是可选文本。
// 输出：返回 SQL 参数。
// 副作用：无。
func nullText(value string) any {
	// 1. 空值不写入无意义文本。
	if value == "" {
		return nil
	}
	return value
}
