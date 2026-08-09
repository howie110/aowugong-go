package subscription

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/howiedata/aowugong-go/internal/database"
)

// Repository 负责订阅记录的 PostgreSQL 持久化。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建订阅仓储。
// 输入：db 是已完成迁移的 PostgreSQL 连接池。
// 输出：返回订阅仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// SeedDefaults 在订阅表为空时写入旧项目的六条默认记录。
// 输入：ctx 是调用上下文。
// 输出：成功返回 nil。
// 副作用：可能写入 PostgreSQL。
func (r *Repository) SeedDefaults(ctx context.Context) error {
	// 1. 使用事务检查空表，避免覆盖页面后续修改。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始订阅初始化事务: %w", err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_record`).Scan(&count); err != nil {
		return fmt.Errorf("检查订阅记录数量: %w", err)
	}
	if count != 0 {
		return tx.Commit()
	}

	// 2. 写入与旧项目一致的初始订阅数据。
	defaults := []WriteRequest{
		{ServiceName: "阿里云服务器", Category: "IT", AnnualFee: "99.00", MonthlyFee: "8.00", StartsOn: "2024-03-07", ExpiresOn: "2027-03-07"},
		{ServiceName: "阿里云域名howie.top", Category: "IT", AnnualFee: "13.00", MonthlyFee: "1.00", ExpiresOn: "2025-01-23"},
		{ServiceName: "阿里云域名aowugong.top", Category: "IT", AnnualFee: "32.00", MonthlyFee: "3.00", StartsOn: "2024-05-10", ExpiresOn: "2027-05-10"},
		{ServiceName: "淘宝88VIP（网易云音乐/优酷）", Category: "生活", AnnualFee: "88.00", MonthlyFee: "7.00", ExpiresOn: "2026-05-24"},
		{ServiceName: "B站", Category: "生活", AnnualFee: "85.00", MonthlyFee: "7.00", ExpiresOn: "2027-03-21"},
		{ServiceName: "腾讯视频VIP", Category: "生活", AnnualFee: "148.00", MonthlyFee: "12.00", ExpiresOn: "2025-09-23"},
	}
	for _, record := range defaults {
		var startsOn any
		if record.StartsOn != "" {
			startsOn = record.StartsOn
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO subscription_record (
				service_name, note, category, annual_fee, monthly_fee, starts_on, expires_on, created_by
			) VALUES (?, '', ?, ?, ?, ?, ?, ?)
		`, record.ServiceName, record.Category, record.AnnualFee, record.MonthlyFee, startsOn, record.ExpiresOn, "邓子豪"); err != nil {
			return fmt.Errorf("写入默认订阅 %s: %w", record.ServiceName, err)
		}
	}

	// 3. 原子提交全部默认记录。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交订阅初始化事务: %w", err)
	}
	return nil
}

// List 返回按到期日和名称排序的全部订阅记录。
// 输入：ctx 是调用上下文。
// 输出：返回数据库原始记录列表。
// 副作用：读取 PostgreSQL。
func (r *Repository) List(ctx context.Context) ([]storedRecord, error) {
	// 1. 执行小表全量查询并确保释放游标。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, service_name, COALESCE(note, ''), category,
		       annual_fee, monthly_fee,
		       starts_on, expires_on, created_by, created_at, updated_at
		FROM subscription_record
		ORDER BY expires_on, service_name
	`)
	if err != nil {
		return nil, fmt.Errorf("查询订阅列表: %w", err)
	}
	defer rows.Close()

	// 2. 扫描订阅原始字段。
	records := make([]storedRecord, 0)
	for rows.Next() {
		record, err := scanStoredRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描订阅记录: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历订阅记录: %w", err)
	}
	return records, nil
}

// Get 按主键返回一条订阅记录。
// 输入：ctx 是调用上下文，recordID 是订阅主键。
// 输出：返回数据库原始记录；不存在时返回 ErrNotFound。
// 副作用：读取 PostgreSQL。
func (r *Repository) Get(ctx context.Context, recordID int64) (storedRecord, error) {
	// 1. 查询单条订阅并复用统一扫描逻辑。
	row := r.db.QueryRowContext(ctx, `
		SELECT id, service_name, COALESCE(note, ''), category,
		       annual_fee, monthly_fee,
		       starts_on, expires_on, created_by, created_at, updated_at
		FROM subscription_record
		WHERE id = ?
	`, recordID)
	record, err := scanStoredRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storedRecord{}, ErrNotFound
	}
	if err != nil {
		return storedRecord{}, fmt.Errorf("查询订阅记录: %w", err)
	}
	return record, nil
}

// Create 新增订阅记录并返回最终数据库记录。
// 输入：ctx 是调用上下文，request 是已清洗字段，createdBy 是创建用户名。
// 输出：返回新记录；服务名冲突时返回 ErrConflict。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) Create(ctx context.Context, request WriteRequest, createdBy string) (storedRecord, error) {
	// 1. 把空开始日期转换为 SQL NULL 并写入记录。
	var startsOn any
	if request.StartsOn != "" {
		startsOn = request.StartsOn
	}
	var recordID int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO subscription_record (
			service_name, note, category, annual_fee, monthly_fee, starts_on, expires_on, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, request.ServiceName, request.Note, request.Category, request.AnnualFee, request.MonthlyFee,
		startsOn, request.ExpiresOn, nullIfEmpty(createdBy)).Scan(&recordID)
	if err != nil {
		if appdatabase.IsDuplicateKey(err) {
			return storedRecord{}, ErrConflict
		}
		return storedRecord{}, fmt.Errorf("新增订阅记录: %w", err)
	}

	// 2. 使用返回主键读取新记录。
	return r.Get(ctx, recordID)
}

// Update 全量更新订阅可编辑字段。
// 输入：ctx 是调用上下文，recordID 是主键，request 是已清洗字段。
// 输出：返回更新后记录；不存在或冲突时返回业务错误。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) Update(ctx context.Context, recordID int64, request WriteRequest) (storedRecord, error) {
	// 1. 全量覆盖可编辑字段并刷新更新时间。
	var startsOn any
	if request.StartsOn != "" {
		startsOn = request.StartsOn
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE subscription_record
		SET service_name = ?, note = ?, category = ?, annual_fee = ?, monthly_fee = ?,
		    starts_on = ?, expires_on = ?, updated_at = ?
		WHERE id = ?
	`, request.ServiceName, request.Note, request.Category, request.AnnualFee, request.MonthlyFee,
		startsOn, request.ExpiresOn, appdatabase.TimestampText(time.Now()), recordID)
	if err != nil {
		if appdatabase.IsDuplicateKey(err) {
			return storedRecord{}, ErrConflict
		}
		return storedRecord{}, fmt.Errorf("更新订阅记录: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storedRecord{}, fmt.Errorf("读取订阅更新结果: %w", err)
	}
	if rowsAffected == 0 {
		return storedRecord{}, ErrNotFound
	}
	return r.Get(ctx, recordID)
}

// Delete 按主键删除订阅记录。
// 输入：ctx 是调用上下文，recordID 是订阅主键。
// 输出：返回是否删除；不存在时返回 ErrNotFound。
// 副作用：写入 PostgreSQL。
func (r *Repository) Delete(ctx context.Context, recordID int64) (bool, error) {
	// 1. 执行主键删除并检查影响行数。
	result, err := r.db.ExecContext(ctx, `DELETE FROM subscription_record WHERE id = ?`, recordID)
	if err != nil {
		return false, fmt.Errorf("删除订阅记录: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取订阅删除结果: %w", err)
	}
	if rowsAffected == 0 {
		return false, ErrNotFound
	}
	return true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanStoredRecord 从数据库行读取订阅字段。
// 输入：scanner 是 QueryRow 或 Rows 的当前行。
// 输出：返回内部记录。
// 副作用：读取数据库游标。
func scanStoredRecord(scanner rowScanner) (storedRecord, error) {
	// 1. 使用 NullString 保留可空字段语义。
	var record storedRecord
	var startsOn, createdBy, createdAt, updatedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.ServiceName,
		&record.Note,
		&record.Category,
		&record.AnnualFee,
		&record.MonthlyFee,
		&startsOn,
		&record.ExpiresOn,
		&createdBy,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return storedRecord{}, err
	}
	record.StartsOn = nullableString(startsOn)
	record.CreatedBy = nullableString(createdBy)
	record.CreatedAt = nullableString(createdAt)
	record.UpdatedAt = nullableString(updatedAt)
	return record, nil
}

// nullableString 把有效的 sql.NullString 转换为字符串指针。
// 输入：value 是数据库可空字符串。
// 输出：有效时返回字符串指针，否则返回 nil。
// 副作用：无。
func nullableString(value sql.NullString) *string {
	// 1. 保留数据库 NULL 与空字符串的差异。
	if !value.Valid {
		return nil
	}
	return &value.String
}

// nullIfEmpty 把空字符串转换为 SQL NULL。
// 输入：value 是可选文本。
// 输出：返回可作为 SQL 参数的字符串或 nil。
// 副作用：无。
func nullIfEmpty(value string) any {
	// 1. 清理文本并转换空值。
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
