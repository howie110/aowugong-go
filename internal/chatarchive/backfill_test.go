package chatarchive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// 使用公司脚本同一协议：历史起点、游标不变、标记随消息一起提交。
func backfillFixture(t *testing.T, previous SyncState) UploadRequest {
	t.Helper()
	u := ingestFixture()
	u.ExpectedState = &previous
	u.State = previous
	u.Batches[0].StartOffset = 20
	u.Batches[0].Messages[0].Position = 20
	u.Batches[0].Messages[0].MessageKey = "id:missing"
	u.Batches[0].Messages[0].Body = "missing text"
	marker := digestString(pythonJSON([]any{int64(20), "id:missing", "missing text"}, false))
	raw, _ := json.Marshal(u)
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	obj["state"].(map[string]any)["native_user_backfill_v1"] = []string{marker}
	raw, _ = json.Marshal(obj)
	json.Unmarshal(raw, &u)
	return u
}
func TestBackfillValidation(t *testing.T) {
	u := backfillFixture(t, ingestFixture().State)
	if err := u.Validate(); err != nil {
		t.Fatal("historical backfill rejected", err)
	}
	u.Batches[0].Messages[0].Body = "tampered"
	if !errors.Is(u.Validate(), ErrInvalidIngest) {
		t.Fatal("marker must bind message")
	}
}
func TestPostgresBackfillRetryRollbackAndIncremental(t *testing.T) {
	db := ingestTestDB(t)
	r := NewRepository(db)
	ctx := context.Background()
	initial := ingestFixture()
	if err := r.Upload(ctx, initial); err != nil {
		t.Fatal(err)
	}
	u := backfillFixture(t, initial.State)
	if _, err := db.Exec(`ALTER TABLE chat_archive_message_versions ADD CONSTRAINT reject_missing CHECK(body<>'missing text')`); err != nil {
		t.Fatal(err)
	}
	if err := r.Upload(ctx, u); err == nil {
		t.Fatal("expected rollback")
	}
	state, _ := r.SyncState(ctx, u.SourceKey)
	raw, _ := json.Marshal(state)
	if string(raw) != mustStateJSON(initial.State) {
		t.Fatal("failed upload changed state")
	}
	db.Exec(`ALTER TABLE chat_archive_message_versions DROP CONSTRAINT reject_missing`)
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("backfill", err)
	}
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("retry", err)
	}
	state, _ = r.SyncState(ctx, u.SourceKey)
	raw, _ = json.Marshal(state)
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	if len(obj["native_user_backfill_v1"].([]any)) != 1 || state.Offset != initial.State.Offset {
		t.Fatal("marker/cursor")
	}
	next := ingestFixture()
	next.ExpectedState = state
	next.State.Offset = 200
	next.Batches[0].StartOffset = 100
	next.Batches[0].Messages[0].Position = 120
	if err := r.Upload(ctx, next); err != nil {
		t.Fatal("regular append", err)
	}
	state, _ = r.SyncState(ctx, u.SourceKey)
	raw, _ = json.Marshal(state)
	json.Unmarshal(raw, &obj)
	if len(obj["native_user_backfill_v1"].([]any)) != 1 {
		t.Fatal("regular sync lost marker")
	}
	if err := r.Upload(ctx, u); !errors.Is(err, ErrIngestConflict) {
		t.Fatal("stale backfill", err)
	}
	result, err := r.VerifyIngest(ctx)
	if err != nil || result.Messages != 3 {
		t.Fatal(result, err)
	}
}
func mustStateJSON(s SyncState) string { b, _ := json.Marshal(s); return string(b) }
