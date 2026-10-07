package notification

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (r *Repository) lookupDispatch(ctx context.Context, source, key, message string) (DispatchResult, bool, error) {
	var result DispatchResult
	var saved string
	err := r.db.QueryRowContext(ctx, `SELECT id, status, message FROM notification_log WHERE source = ? AND request_id = ?`, source, key).Scan(&result.ID, &result.Status, &saved)
	if errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, ErrStorage
	}
	if saved != message {
		return result, true, ErrConflict
	}
	result.Duplicate = true
	return result, true, nil
}

// claimDispatch 通过唯一索引保证多进程或并发调用也只能占位一次。
func (r *Repository) claimDispatch(ctx context.Context, source, key, title, message string) (DispatchResult, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO notification_log(channel,title,message,status,error_message,sent_at,source,request_id)
	VALUES(?,?,?,'unknown',?,?,?,?) ON CONFLICT(source,request_id) DO NOTHING`,
		"wecom_bot", title, message, "发送中或结果未确认；禁止自动重发", time.Now(), source, key)
	if err != nil {
		return DispatchResult{}, ErrStorage
	}
	n, err := res.RowsAffected()
	if err != nil {
		return DispatchResult{}, ErrStorage
	}
	result, _, err := r.lookupDispatch(ctx, source, key, message)
	result.Duplicate = n == 0
	return result, err
}

func (r *Repository) finishDispatch(ctx context.Context, id int64, status, detail string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE notification_log SET status = ?, error_message = ? WHERE id = ?`, status, nullIfEmpty(detail), id)
	return err
}
