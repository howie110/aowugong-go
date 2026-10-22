package chatarchive

import (
	"context"
	"strings"
	"time"
)

type AuditFilter struct {
	Source, From, To, Query, ConversationID, MessageID, Order string
	Limit, Offset                                             int
	Snapshot                                                  int64
}

func (f AuditFilter) Validate() error {
	if err := (Filter{Source: f.Source, From: f.From, To: f.To, Query: f.Query, Limit: f.Limit, Offset: f.Offset}).Validate(); err != nil {
		return err
	}
	if (f.Order != "" && f.Order != "asc" && f.Order != "desc") || f.Snapshot < 0 || len(f.ConversationID) > 1000 || len(f.MessageID) > 1000 {
		return ErrInvalidFilter
	}
	return nil
}

type AuditMessage struct {
	ID         int64   `json:"id"`
	MessageKey string  `json:"message_key"`
	Role       string  `json:"role"`
	Body       string  `json:"body"`
	SourceTime *string `json:"source_time"`
	Source     string  `json:"source"`
	SourceID   string  `json:"source_id"`
	DeviceID   string  `json:"device_id"`
	ObservedAt string  `json:"observed_at"`
	parsedTime *time.Time
}

type AuditCounts struct {
	Conversations    int64 `json:"conversations"`
	Messages         int64 `json:"messages"`
	FailedSyncStates int64 `json:"failed_sync_states"`
}

type AuditPage struct {
	Messages []AuditMessage `json:"messages"`
	Counts   AuditCounts    `json:"counts"`
	Total    int            `json:"total"`
	HasMore  bool           `json:"has_more"`
	Snapshot int64          `json:"snapshot"`
}

// 先按身份选最新观察，只在数据库完成排序分页后读取当前页正文。
const latestAudit = `WITH latest AS MATERIALIZED (
 SELECT DISTINCT ON (conversation_id,message_key) id,conversation_id,message_key,role,source_at
 FROM chat_archive_message_versions WHERE id<=? ORDER BY conversation_id,message_key,id DESC) `

func (r *Repository) ListAudit(ctx context.Context, f AuditFilter) (AuditPage, error) {
	result := AuditPage{Messages: []AuditMessage{}}
	if err := f.Validate(); err != nil {
		return result, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM chat_archive_conversations),
 (SELECT count(*) FROM (SELECT DISTINCT conversation_id,message_key FROM chat_archive_message_versions) identities),
 (SELECT count(*) FROM chat_archive_sync_states WHERE NULLIF(last_error,'') IS NOT NULL AND (last_success_at IS NULL OR last_failure_at IS NULL OR last_failure_at>=last_success_at)),
 (SELECT COALESCE(max(id),0) FROM chat_archive_message_versions)`).Scan(&result.Counts.Conversations, &result.Counts.Messages, &result.Counts.FailedSyncStates, &result.Snapshot); err != nil {
		return result, err
	}
	if f.Snapshot > 0 {
		result.Snapshot = f.Snapshot
	}
	from := ` FROM latest m JOIN chat_archive_conversations c ON c.id=m.conversation_id WHERE TRUE`
	args := []any{result.Snapshot}
	for _, c := range []struct{ sql, value string }{
		{" AND c.source=?", f.Source}, {" AND c.source_id=?", f.ConversationID}, {" AND m.message_key=?", f.MessageID},
		{" AND m.source_at>=?::date AT TIME ZONE 'UTC'", f.From}, {" AND m.source_at<(?::date+interval '1 day') AT TIME ZONE 'UTC'", f.To},
	} {
		if c.value != "" {
			from += c.sql
			args = append(args, c.value)
		}
	}
	if f.Query != "" {
		from += ` AND EXISTS(SELECT 1 FROM chat_archive_message_versions v WHERE v.id=m.id AND v.body ILIKE ? ESCAPE '\')`
		args = append(args, likeTerm(f.Query))
	}
	if err := r.db.QueryRowContext(ctx, latestAudit+`SELECT count(*)`+from, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	order := "DESC"
	if f.Order == "asc" {
		order = "ASC"
	}
	query := latestAudit + `, page AS (SELECT m.id,m.source_at` + from + ` ORDER BY m.source_at ` + order + ` NULLS LAST,m.id ASC LIMIT ? OFFSET ?)
 SELECT v.id,v.message_key,v.role,v.body,v.source_time,c.source,c.source_id,b.device_id,b.observed_at::text
 FROM page p JOIN chat_archive_message_versions v ON v.id=p.id
 JOIN chat_archive_conversations c ON c.id=v.conversation_id JOIN chat_archive_raw_batches b ON b.id=v.batch_id
 ORDER BY p.source_at ` + order + ` NULLS LAST,p.id ASC`
	rows, err := r.db.QueryContext(ctx, query, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var m AuditMessage
		if err := rows.Scan(&m.ID, &m.MessageKey, &m.Role, &m.Body, &m.SourceTime, &m.Source, &m.SourceID, &m.DeviceID, &m.ObservedAt); err != nil {
			return result, err
		}
		parsed := parseSourceTime(m.SourceTime)
		m.SourceTime = nil
		if parsed != nil {
			s := parsed.Format(time.RFC3339Nano)
			m.SourceTime = &s
		}
		result.Messages = append(result.Messages, m)
	}
	result.HasMore = f.Offset+len(result.Messages) < result.Total
	return result, rows.Err()
}

func parseSourceTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(*value)); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}
