// Package notification 提供统一业务通知、OpeniLink 调用和发送日志。
package notification

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository 负责通知发送结果的 MySQL 持久化。
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

// NewRepository 创建通知日志仓储。
// 输入：db 是已完成迁移的 MySQL 连接。
// 输出：返回可并发复用的仓储。
// 副作用：无，不执行 SQL。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存 MySQL 依赖供每次发送结果写入。
	return &Repository{db: db}
}

// Save 写入一条通知发送结果。
// 输入：ctx 控制写入，record 包含渠道、标题、正文、状态和错误。
// 输出：成功返回 nil，写入失败返回错误。
// 副作用：向 MySQL notification_log 新增一行。
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
