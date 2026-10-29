// Package notification 提供统一业务通知、企业微信调用和发送日志。
package notification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Repository 负责通知发送结果的 PostgreSQL 持久化。
type Repository struct {
	db *sql.DB
}

// Record 描述一条待写入的通知日志。
type Record struct {
	Channel      string
	Title        string
	Message      string
	Status       string
	ErrorMessage string
}

// LogFilter 描述通知历史的筛选和分页。
type LogFilter struct {
	Query    string
	Status   string
	Category string
	Page     int
	PageSize int
}

// LogEntry 描述一条通知历史。
type LogEntry struct {
	ID           int64  `json:"id"`
	Channel      string `json:"channel"`
	Title        string `json:"title"`
	Message      string `json:"message"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	SentAt       string `json:"sent_at"`
	Source       string `json:"source"`
	RequestID    string `json:"request_id"`
}

// LogPage 描述一页通知历史。
type LogPage struct {
	Items    []LogEntry `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// NewRepository 创建通知日志仓储。
// 输入：db 是已完成迁移的 PostgreSQL 连接。
// 输出：返回可并发复用的仓储。
// 副作用：无，不执行 SQL。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存 PostgreSQL 依赖供每次发送结果写入。
	return &Repository{db: db}
}

// Save 写入一条通知发送结果。
// 输入：ctx 控制写入，record 包含渠道、标题、正文、状态和错误。
// 输出：成功返回 nil，写入失败返回错误。
// 副作用：向 PostgreSQL notification_log 新增一行。
func (r *Repository) Save(ctx context.Context, record Record) error {
	// 1. 使用统一时间格式写入完整发送结果。
	_, err := r.db.ExecContext(ctx, `INSERT INTO notification_log(
		channel, title, message, status, error_message, sent_at
	) VALUES(?,?,?,?,?,?)`, record.Channel, record.Title, record.Message, record.Status,
		nullIfEmpty(record.ErrorMessage), time.Now())
	if err != nil {
		return fmt.Errorf("写入通知日志: %w", err)
	}
	return nil
}

// nullIfEmpty 把空错误文本转换为 SQL NULL。
// 输入：value 是可选错误文本。
// 输出：空值返回 nil，非空值返回原文本。
// 副作用：无。
func nullIfEmpty(value string) any {
	// 1. 保持成功日志的 error_message 为 NULL。
	if value == "" {
		return nil
	}
	return value
}

// List 查询通知发送历史。
// 输入：ctx 控制查询，filter 提供关键字、状态、分类和分页。
// 输出：返回当前页、总数和分页信息。
// 副作用：只读 PostgreSQL notification_log。
func (r *Repository) List(ctx context.Context, filter LogFilter) (LogPage, error) {
	// 1. 规范化分页参数，避免负数或超大页。
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	where, args := notificationLogWhere(filter)

	// 2. 统计总数并读取当前页。
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_log WHERE "+where, args...).Scan(&total); err != nil {
		return LogPage{}, fmt.Errorf("统计通知历史: %w", err)
	}
	query := `SELECT id, channel, title, message, status,
		COALESCE(error_message, ''), sent_at, COALESCE(source, ''), COALESCE(request_id, '')
		FROM notification_log WHERE ` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return LogPage{}, fmt.Errorf("查询通知历史: %w", err)
	}
	defer rows.Close()

	items := make([]LogEntry, 0, filter.PageSize)
	for rows.Next() {
		var item LogEntry
		if err := rows.Scan(
			&item.ID, &item.Channel, &item.Title, &item.Message, &item.Status,
			&item.ErrorMessage, &item.SentAt, &item.Source, &item.RequestID,
		); err != nil {
			return LogPage{}, fmt.Errorf("扫描通知历史: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return LogPage{}, fmt.Errorf("遍历通知历史: %w", err)
	}
	return LogPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

// notificationLogWhere 构造通知历史筛选条件。
// 输入：filter 提供关键字、状态和分类。
// 输出：返回 WHERE 子句和参数。
// 副作用：无。
func notificationLogWhere(filter LogFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 6)
	if query := strings.TrimSpace(filter.Query); query != "" {
		like := "%" + query + "%"
		clauses = append(clauses, "(LOWER(title) LIKE LOWER(?) OR LOWER(message) LIKE LOWER(?))")
		args = append(args, like, like)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	switch filter.Category {
	case "任务":
		clauses = append(clauses, "title LIKE ?")
		args = append(args, "任务 / %")
	case "备份":
		clauses = append(clauses, "(title LIKE ? OR title LIKE ?)")
		args = append(args, "备份 / %", "server-alerts / Vaultwarden 备份%")
	case "监控":
		clauses = append(clauses, "title LIKE ?")
		args = append(args, "监控 / %")
	case "订阅":
		clauses = append(clauses, "title LIKE ?")
		args = append(args, "订阅 / %")
	case "项目":
		clauses = append(clauses, "source IS NOT NULL AND title NOT LIKE ?")
		args = append(args, "server-alerts / Vaultwarden 备份%")
	case "其他":
		clauses = append(clauses,
			"source IS NULL AND title NOT LIKE ? AND title NOT LIKE ? AND title NOT LIKE ? AND title NOT LIKE ?")
		args = append(args, "任务 / %", "备份 / %", "监控 / %", "订阅 / %")
	}
	return strings.Join(clauses, " AND "), args
}
