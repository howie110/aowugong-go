package chatarchive

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type CheckMessage struct {
	MessageKey  string `json:"message_key"`
	VersionHash string `json:"version_hash"`
}
type CheckRequest struct {
	Source         string         `json:"source"`
	AccountScope   string         `json:"account_scope"`
	ConversationID string         `json:"conversation_id"`
	Messages       []CheckMessage `json:"messages"`
}
type CheckResult struct {
	Statuses []string `json:"statuses"`
}
type RepairRequest struct {
	SourceKey      string          `json:"source_key"`
	DeviceID       string          `json:"device_id"`
	AccountScope   string          `json:"account_scope"`
	Source         string          `json:"source"`
	SourcePath     string          `json:"source_path"`
	ConversationID string          `json:"conversation_id"`
	Messages       []IngestMessage `json:"messages"`
}
type RepairResult struct {
	Inserted int64 `json:"inserted"`
	Verified int64 `json:"verified"`
}

func (c CheckRequest) Validate() error {
	for _, check := range []struct {
		field string
		valid bool
	}{
		{"source", c.Source == "codex" || c.Source == "claude-code"},
		{"account_scope", ingestText(c.AccountScope, 1000)},
		{"conversation_id", ingestText(c.ConversationID, 1000)},
		{"messages", c.Messages != nil && len(c.Messages) <= 500},
	} {
		if !check.valid {
			return invalidIngestField(check.field)
		}
	}
	for i, m := range c.Messages {
		if !ingestText(m.MessageKey, 2000) || !ValidSourceKey(m.VersionHash) {
			return invalidIngestField(fmt.Sprintf("messages[%d]", i))
		}
	}
	return nil
}

func (r RepairRequest) Validate() error {
	u := UploadRequest{SourceKey: r.SourceKey, DeviceID: r.DeviceID, AccountScope: r.AccountScope, Source: r.Source, SourcePath: r.SourcePath}
	c := CheckRequest{Source: r.Source, AccountScope: r.AccountScope, ConversationID: r.ConversationID, Messages: []CheckMessage{}}
	if err := c.Validate(); err != nil {
		return err
	}
	for _, check := range []struct {
		field string
		valid bool
	}{
		{"source_key", ValidSourceKey(r.SourceKey)},
		{"device_id", ingestText(r.DeviceID, 1000)},
		{"source_path", ingestText(r.SourcePath, 8192) && absoluteIngestPath(r.SourcePath)},
		{"source_key.hash", sourceKey(u) == r.SourceKey},
		{"messages", r.Messages != nil && len(r.Messages) <= 500},
	} {
		if !check.valid {
			return invalidIngestField(check.field)
		}
	}
	prev := int64(-1)
	for i, m := range r.Messages {
		if !m.valid() {
			return invalidIngestField(fmt.Sprintf("messages[%d].fields", i))
		}
		if m.Position <= prev {
			return invalidIngestField(fmt.Sprintf("messages[%d].position", i))
		}
		prev = m.Position
	}
	return nil
}

// Check returns only membership in the logical conversation, including all historical versions.
func (r *Repository) Check(ctx context.Context, c CheckRequest) (CheckResult, error) {
	if err := c.Validate(); err != nil {
		return CheckResult{}, err
	}
	return checkVersions(ctx, r.db, c)
}
func checkVersions(ctx context.Context, q ingestQuerier, c CheckRequest) (CheckResult, error) {
	result := CheckResult{Statuses: make([]string, 0, len(c.Messages))}
	raw, _ := json.Marshal(c.Messages)
	rows, err := q.QueryContext(ctx, `WITH conversation AS (
 SELECT id FROM chat_archive_conversations WHERE source=? AND account_scope=? AND source_id=?
 ) SELECT CASE
 WHEN EXISTS(SELECT 1 FROM chat_archive_message_versions m JOIN conversation c ON c.id=m.conversation_id WHERE m.message_key=p.value->>'message_key' AND m.version_hash=p.value->>'version_hash') THEN 'present'
 WHEN EXISTS(SELECT 1 FROM chat_archive_message_versions m JOIN conversation c ON c.id=m.conversation_id WHERE m.message_key=p.value->>'message_key') THEN 'different'
 ELSE 'missing' END
 FROM jsonb_array_elements(?::jsonb) WITH ORDINALITY AS p(value,ordinal) ORDER BY ordinal`, c.Source, c.AccountScope, c.ConversationID, string(raw))
	if err != nil {
		return CheckResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return CheckResult{}, err
		}
		result.Statuses = append(result.Statuses, status)
	}
	return result, rows.Err()
}

// Repair appends missing versions without creating or changing any sync state.
// Lock order is source first (shared with Upload), then logical conversation
// (shared by repairs from different devices). No existing observation is changed.
func (r *Repository) Repair(ctx context.Context, u RepairRequest) (RepairResult, error) {
	if err := u.Validate(); err != nil {
		return RepairResult{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RepairResult{}, err
	}
	defer tx.Rollback()
	for _, statement := range []string{`SET LOCAL lock_timeout='10s'`, `SET LOCAL statement_timeout='30s'`} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return RepairResult{}, err
		}
	}
	key, _ := hex.DecodeString(u.SourceKey)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, int64(binary.BigEndian.Uint64(key[:8]))); err != nil {
		return RepairResult{}, err
	}
	// The two-int namespace is separate from Upload's bigint source lock namespace.
	conversationKey := digestString(pythonJSON([]string{u.Source, u.AccountScope, u.ConversationID}, true))
	key, _ = hex.DecodeString(conversationKey)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?::integer,?::integer)`, int32(binary.BigEndian.Uint32(key[:4])), int32(binary.BigEndian.Uint32(key[4:8]))); err != nil {
		return RepairResult{}, err
	}
	var mismatch bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chat_archive_raw_batches b JOIN chat_archive_conversations c ON c.id=b.conversation_id WHERE b.source_key=? AND (c.source<>? OR c.account_scope<>? OR c.source_id<>? OR b.device_id<>?)) OR EXISTS(SELECT 1 FROM chat_archive_sync_states WHERE source_key=? AND state IS NOT NULL AND state<>'null'::jsonb AND state->>'conversation_id' IS DISTINCT FROM ?)`, u.SourceKey, u.Source, u.AccountScope, u.ConversationID, u.DeviceID, u.SourceKey, u.ConversationID).Scan(&mismatch)
	if err != nil {
		return RepairResult{}, err
	}
	if mismatch {
		return RepairResult{}, ErrIngestConflict
	}
	c := CheckRequest{Source: u.Source, AccountScope: u.AccountScope, ConversationID: u.ConversationID, Messages: make([]CheckMessage, 0, len(u.Messages))}
	for _, m := range u.Messages {
		c.Messages = append(c.Messages, CheckMessage{m.MessageKey, messageHash(m)})
	}
	checked, err := checkVersions(ctx, tx, c)
	if err != nil {
		return RepairResult{}, err
	}
	missing := []IngestMessage{}
	seen := map[CheckMessage]bool{}
	for i, m := range u.Messages {
		identity := c.Messages[i]
		if checked.Statuses[i] != "present" && !seen[identity] {
			missing = append(missing, m)
		}
		seen[identity] = true
	}
	if len(missing) > 0 {
		title := u.ConversationID
		for _, m := range u.Messages {
			if m.Role == "user" {
				runes := []rune(m.Body)
				title = string(runes[:min(len(runes), 120)])
				break
			}
		}
		var cid, bid int64
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_archive_conversations(source,account_scope,source_id,title) VALUES(?,?,?,?) ON CONFLICT(source,account_scope,source_id) DO UPDATE SET updated_at=now(),title=CASE WHEN chat_archive_conversations.title=chat_archive_conversations.source_id THEN EXCLUDED.title ELSE chat_archive_conversations.title END RETURNING id`, u.Source, u.AccountScope, u.ConversationID, title).Scan(&cid)
		if err != nil {
			return RepairResult{}, err
		}
		hash := batchHash(missing)
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_archive_raw_batches(conversation_id,source_key,device_id,source_path,start_offset,raw,sha256,parser_version,unknown_events) VALUES(?,?,?,?,?,?,?,?,0) RETURNING id`, cid, u.SourceKey, u.DeviceID, u.SourcePath, missing[0].Position, []byte{}, hash, IngestParserVersion).Scan(&bid)
		if err != nil {
			return RepairResult{}, err
		}
		for _, m := range missing {
			_, err = tx.ExecContext(ctx, `INSERT INTO chat_archive_message_versions(conversation_id,message_key,version_hash,batch_id,position,parent_key,identity_quality,role,body,blocks,attachments,completeness,source_time) VALUES(?,?,?,?,?,?,?,?,?,'[]','[]',?,?)`, cid, m.MessageKey, messageHash(m), bid, m.Position, m.ParentKey, m.IdentityQuality, m.Role, m.Body, m.Completeness, m.SourceTime)
			if err != nil {
				return RepairResult{}, err
			}
		}
		if _, err = readIngestMessages(ctx, tx, storedIngestBatch{id: bid, conversationID: cid, start: missing[0].Position, hash: hash, version: IngestParserVersion}); err != nil {
			return RepairResult{}, err
		}
	}
	verified, err := checkVersions(ctx, tx, c)
	if err != nil {
		return RepairResult{}, err
	}
	for _, status := range verified.Statuses {
		if status != "present" {
			return RepairResult{}, ErrArchiveIntegrity
		}
	}
	if err = tx.Commit(); err != nil {
		return RepairResult{}, err
	}
	return RepairResult{Inserted: int64(len(missing)), Verified: int64(len(verified.Statuses))}, nil
}
