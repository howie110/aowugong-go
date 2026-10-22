package chatarchive

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

type ingestQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSyncState(ctx context.Context, q ingestQuerier, key string) (*SyncState, error) {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT state FROM chat_archive_sync_states WHERE source_key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || err == nil && len(raw) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state *SyncState
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, ErrArchiveIntegrity
	}
	return state, nil
}
func (r *Repository) SyncState(ctx context.Context, key string) (*SyncState, error) {
	if !ValidSourceKey(key) {
		return nil, ErrInvalidIngest
	}
	return readSyncState(ctx, r.db, key)
}
func legacyCounts(ctx context.Context, q ingestQuerier) (LegacyCounts, error) {
	var result LegacyCounts
	err := q.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM chat_archive_raw_batches WHERE parser_version<>? OR octet_length(raw)>0),
 (SELECT count(*) FROM chat_archive_message_versions WHERE role NOT IN ('user','assistant') OR blocks<>'[]'::jsonb OR attachments<>'[]'::jsonb OR completeness NOT IN ('source_text','escaped_source_text'))`, IngestParserVersion).Scan(&result.Batches, &result.Messages)
	return result, err
}
func (r *Repository) LegacyCounts(ctx context.Context) (LegacyCounts, error) {
	return legacyCounts(ctx, r.db)
}

// Upload holds the same source-key lock as the Python writer, including the first insert.
// A source rescan starts at zero; CAS still compares the previous committed state.
func (r *Repository) Upload(ctx context.Context, u UploadRequest) error {
	if err := u.Validate(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout='10s'`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `SET LOCAL statement_timeout='30s'`); err != nil {
		return err
	}
	key, _ := hex.DecodeString(u.SourceKey)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, int64(binary.BigEndian.Uint64(key[:8]))); err != nil {
		return err
	}
	current, err := readSyncState(ctx, tx, u.SourceKey)
	if err != nil {
		return err
	}
	// 普通旧客户端构造的新 state 不带补传字段，保留它读到的标记。
	// 仍使用完整 expected_state 做 CAS，不能覆盖并发补传。
	if len(u.State.BackfillMarkers) == 0 && u.ExpectedState != nil {
		u.State.BackfillMarkers = u.ExpectedState.BackfillMarkers
	}
	if reflect.DeepEqual(current, &u.State) {
		match, err := exactIngestRetry(ctx, tx, u)
		if err != nil {
			return err
		}
		if !match {
			return ErrIngestConflict
		}
		return tx.Commit()
	}
	if !reflect.DeepEqual(current, u.ExpectedState) {
		return ErrIngestConflict
	}
	legacy, err := legacyCounts(ctx, tx)
	if err != nil {
		return err
	}
	if legacy.Batches != 0 || legacy.Messages != 0 {
		return ErrIngestConflict
	}
	// A key may never switch the conversation or account/source metadata already archived.
	var mismatch bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chat_archive_raw_batches b JOIN chat_archive_conversations c ON c.id=b.conversation_id WHERE b.source_key=? AND (c.source<>? OR c.account_scope<>? OR c.source_id<>? OR b.device_id<>?))`, u.SourceKey, u.Source, u.AccountScope, u.State.ConversationID, u.DeviceID).Scan(&mismatch)
	if err != nil {
		return err
	}
	if mismatch {
		return ErrIngestConflict
	}
	for _, b := range u.Batches {
		title := b.ConversationID
		for _, m := range b.Messages {
			if m.Role == "user" {
				runes := []rune(m.Body)
				title = string(runes[:min(len(runes), 120)])
				break
			}
		}
		var cid, bid int64
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_archive_conversations(source,account_scope,source_id,title) VALUES(?,?,?,?) ON CONFLICT(source,account_scope,source_id) DO UPDATE SET updated_at=now(),title=CASE WHEN chat_archive_conversations.title=chat_archive_conversations.source_id THEN EXCLUDED.title ELSE chat_archive_conversations.title END RETURNING id`, u.Source, u.AccountScope, b.ConversationID, title).Scan(&cid)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_archive_raw_batches(conversation_id,source_key,device_id,source_path,start_offset,raw,sha256,parser_version,unknown_events) VALUES(?,?,?,?,?,?,?,?,?) RETURNING id`, cid, u.SourceKey, u.DeviceID, u.SourcePath, b.StartOffset, []byte{}, batchHash(b.Messages), IngestParserVersion, b.UnknownEvents).Scan(&bid)
		if err != nil {
			return err
		}
		for _, m := range b.Messages {
			_, err = tx.ExecContext(ctx, `INSERT INTO chat_archive_message_versions(conversation_id,message_key,version_hash,batch_id,position,parent_key,identity_quality,role,body,blocks,attachments,completeness,source_time) VALUES(?,?,?,?,?,?,?,?,?,'[]','[]',?,?)`, cid, m.MessageKey, messageHash(m), bid, m.Position, m.ParentKey, m.IdentityQuality, m.Role, m.Body, m.Completeness, m.SourceTime)
			if err != nil {
				return err
			}
		}
	}
	state, _ := json.Marshal(u.State)
	_, err = tx.ExecContext(ctx, `INSERT INTO chat_archive_sync_states(source_key,state,last_success_at) VALUES(?,?::jsonb,now()) ON CONFLICT(source_key) DO UPDATE SET state=EXCLUDED.state,last_success_at=now(),last_error=NULL`, u.SourceKey, string(state))
	if err != nil {
		return err
	}
	return tx.Commit()
}

type storedIngestBatch struct {
	id                                                         int64
	conversationID                                             int64
	key, device, path, source, account, session, hash, version string
	start, unknown                                             int64
	raw                                                        []byte
}

func readIngestBatches(ctx context.Context, q ingestQuerier, where string, args ...any) ([]storedIngestBatch, error) {
	rows, err := q.QueryContext(ctx, `SELECT b.id,b.conversation_id,b.source_key,b.device_id,b.source_path,c.source,c.account_scope,c.source_id,b.sha256,b.parser_version,b.start_offset,b.unknown_events,b.raw FROM chat_archive_raw_batches b JOIN chat_archive_conversations c ON c.id=b.conversation_id `+where+` ORDER BY b.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []storedIngestBatch
	for rows.Next() {
		var b storedIngestBatch
		if err = rows.Scan(&b.id, &b.conversationID, &b.key, &b.device, &b.path, &b.source, &b.account, &b.session, &b.hash, &b.version, &b.start, &b.unknown, &b.raw); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}
func readIngestMessages(ctx context.Context, q ingestQuerier, b storedIngestBatch) ([]IngestMessage, error) {
	rows, err := q.QueryContext(ctx, `SELECT conversation_id,message_key,parent_key,identity_quality,role,body,blocks,attachments,completeness,source_time,position,version_hash FROM chat_archive_message_versions WHERE batch_id=? ORDER BY position`, b.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []IngestMessage{}
	for rows.Next() {
		var m IngestMessage
		var cid int64
		var blocks, attachments []byte
		var hash string
		if err = rows.Scan(&cid, &m.MessageKey, &m.ParentKey, &m.IdentityQuality, &m.Role, &m.Body, &blocks, &attachments, &m.Completeness, &m.SourceTime, &m.Position, &hash); err != nil {
			return nil, err
		}
		if json.Unmarshal(blocks, &m.Blocks) != nil || json.Unmarshal(attachments, &m.Attachments) != nil || cid != b.conversationID || !m.valid() || m.Position < b.start || hash != messageHash(m) {
			return nil, ErrArchiveIntegrity
		}
		messages = append(messages, m)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if b.version != IngestParserVersion || len(b.raw) != 0 || batchHash(messages) != b.hash {
		return nil, ErrArchiveIntegrity
	}
	return messages, nil
}
func canonicalIngestPath(path string) string {
	if strings.HasSuffix(path, ".jsonl.zst") {
		return strings.TrimSuffix(path, ".zst")
	}
	return path
}

func exactIngestRetry(ctx context.Context, tx *sql.Tx, u UploadRequest) (bool, error) {
	// now() is constant within a transaction, identifying the complete last upload
	// without changing the Python state shape or requiring a schema migration.
	// repair 不改变同步游标；它追加的新批次不能取代最后一次游标提交的重试凭据。
	batches, err := readIngestBatches(ctx, tx, `WHERE b.source_key=? AND b.observed_at=(SELECT last_success_at FROM chat_archive_sync_states WHERE source_key=?)`, u.SourceKey, u.SourceKey)
	if err != nil {
		return false, err
	}
	if len(batches) != len(u.Batches) {
		return false, nil
	}
	for i, b := range batches {
		want := u.Batches[i]
		if b.device != u.DeviceID || canonicalIngestPath(b.path) != canonicalIngestPath(u.SourcePath) || b.source != u.Source || b.account != u.AccountScope || b.session != want.ConversationID || b.start != want.StartOffset || b.unknown != want.UnknownEvents || b.hash != batchHash(want.Messages) {
			return false, nil
		}
		actual, err := readIngestMessages(ctx, tx, b)
		if err != nil {
			return false, err
		}
		if !reflect.DeepEqual(actual, want.Messages) {
			return false, nil
		}
	}
	return true, nil
}
func (r *Repository) VerifyIngest(ctx context.Context) (VerifyResult, error) {
	result := VerifyResult{}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	batches, err := readIngestBatches(ctx, tx, "")
	if err != nil {
		return result, err
	}
	for _, b := range batches {
		messages, err := readIngestMessages(ctx, tx, b)
		if err != nil {
			return result, err
		}
		result.Batches++
		result.Messages += int64(len(messages))
	}
	if err = tx.Commit(); err != nil {
		return VerifyResult{}, err
	}
	result.Verified = true
	return result, nil
}
