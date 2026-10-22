package chatarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// ListConversations 的日期按归档观察时间筛选，来源时间不一定可用。
func (r *Repository) ListConversations(ctx context.Context, f Filter) ([]Conversation, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	query := `SELECT c.id,c.source,c.source_id,c.title,c.created_at::text,c.updated_at::text,c.account_scope FROM chat_archive_conversations c WHERE TRUE`
	args := []any{}
	if f.Source != "" {
		query += " AND c.source=?"
		args = append(args, f.Source)
	}
	if f.From != "" {
		query += " AND c.updated_at>=?::date"
		args = append(args, f.From)
	}
	if f.To != "" {
		query += " AND c.updated_at<(?::date + interval '1 day')"
		args = append(args, f.To)
	}
	if f.Title != "" {
		query += " AND c.title ILIKE ? ESCAPE '\\'"
		args = append(args, likeTerm(f.Title))
	}
	if f.Query != "" {
		query += " AND EXISTS (SELECT 1 FROM chat_archive_message_versions m WHERE m.conversation_id=c.id AND m.body ILIKE ? ESCAPE '\\')"
		args = append(args, likeTerm(f.Query))
	}
	query += " ORDER BY c.updated_at DESC,c.id DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Source, &c.SourceID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.AccountScope); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func likeTerm(value string) string {
	return "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value) + "%"
}

func (r *Repository) exists(ctx context.Context, id int64) error {
	var found int64
	return r.db.QueryRowContext(ctx, "SELECT id FROM chat_archive_conversations WHERE id=?", id).Scan(&found)
}

const messageColumns = `id,message_key,version_hash,parent_key,role,body,blocks,attachments,completeness,identity_quality,source_time,batch_id,position`

// ListMessages 以首次观察顺序为稳定游标，同一消息展示最近观察版本，不伪造来源顺序。
func (r *Repository) ListMessages(ctx context.Context, id, afterID int64, limit int) ([]Message, error) {
	if id < 1 || afterID < 0 || limit < 1 || limit > 200 {
		return nil, ErrInvalidFilter
	}
	if err := r.exists(ctx, id); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `WITH latest AS (
 SELECT DISTINCT ON(message_key) min(id) OVER(PARTITION BY message_key) AS first_id, `+messageColumns+`
 FROM chat_archive_message_versions WHERE conversation_id=? ORDER BY message_key,id DESC)
 SELECT first_id,message_key,version_hash,parent_key,role,body,blocks,attachments,completeness,identity_quality,source_time,batch_id,position
 FROM latest WHERE first_id>? ORDER BY first_id LIMIT ?`, id, afterID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// ListVersions 保留来源观察（同一内容可能在多设备重复观察），通过 version_hash 识别内容版本。
func (r *Repository) ListVersions(ctx context.Context, id int64, key string, afterID int64, limit int) ([]Message, error) {
	if id < 1 || key == "" || len(key) > 1000 || afterID < 0 || limit < 1 || limit > 200 {
		return nil, ErrInvalidFilter
	}
	if err := r.exists(ctx, id); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+messageColumns+" FROM chat_archive_message_versions WHERE conversation_id=? AND message_key=? AND id>? ORDER BY id LIMIT ?", id, key, afterID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	result := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.MessageKey, &m.VersionHash, &m.ParentKey, &m.Role, &m.Body, &m.Blocks, &m.Attachments, &m.Completeness, &m.IdentityQuality, &m.SourceTime, &m.BatchID, &m.Position); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// Export 流式输出；统一导出包含全部观察版本，raw 保留原始字节的 base64 和采集来源。
func (r *Repository) Export(ctx context.Context, id int64, format string, w io.Writer) error {
	if format != "raw" && format != "jsonl" {
		return ErrInvalidFilter
	}
	if err := r.exists(ctx, id); err != nil {
		return err
	}
	var c Conversation
	if err := r.db.QueryRowContext(ctx, "SELECT id,source,source_id,title,created_at::text,updated_at::text,account_scope FROM chat_archive_conversations WHERE id=?", id).Scan(&c.ID, &c.Source, &c.SourceID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.AccountScope); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(map[string]any{"kind": "conversation", "schema_version": 1, "conversation": c}); err != nil {
		return err
	}
	query := `SELECT to_jsonb(m) FROM chat_archive_message_versions m WHERE conversation_id=? ORDER BY id`
	if format == "raw" {
		query = `SELECT jsonb_build_object('id',id,'conversation_id',conversation_id,'source_key',source_key,'device_id',device_id,'source_path',source_path,'start_offset',start_offset,'sha256',sha256,'parser_version',parser_version,'unknown_events',unknown_events,'observed_at',observed_at,'raw_base64',encode(raw,'base64')) FROM chat_archive_raw_batches WHERE conversation_id=? ORDER BY id`
	}
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		if err := encoder.Encode(map[string]any{"kind": fmt.Sprintf("archive_%s", format), "record": raw}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return encoder.Encode(map[string]any{"kind": "export_complete"})
}
