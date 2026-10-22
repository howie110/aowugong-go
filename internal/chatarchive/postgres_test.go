package chatarchive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// 此测试只接受显式临时数据库，在独立 schema 内创建表，不读取项目配置。
func TestPostgresArchiveQueriesAndExport(t *testing.T) {
	dsn := os.Getenv("CHAT_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN not supplied")
	}
	ctx := context.Background()
	db, err := database.OpenPostgres(ctx, config.Database{URL: dsn, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow("SELECT current_database()").Scan(&name); err != nil || name != "chat_archive_test" {
		t.Fatal("refusing non-test database")
	}
	if _, err := db.Exec("CREATE SCHEMA chat_archive_go_test"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA chat_archive_go_test CASCADE")
	if _, err := db.Exec("SET search_path=chat_archive_go_test"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/postgres/00013_chat_archive.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	performanceMigration, err := os.ReadFile("../../migrations/postgres/00014_chat_archive_time.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(strings.Split(string(performanceMigration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_archive_conversations(source,account_scope,source_id,title) VALUES('codex','account','session','测试标题')`); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\"synthetic\":true}\n")
	sum := sha256.Sum256(raw)
	if _, err := db.Exec(`INSERT INTO chat_archive_raw_batches(conversation_id,source_key,device_id,source_path,start_offset,raw,sha256,parser_version) VALUES(1,'key','device','/synthetic',0,?,?, '1')`, raw, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ key, body, hash string }{{"id:a", "old", "v1"}, {"id:b", "100% literal", "v2"}, {"id:a", "new", "v3"}} {
		if _, err := db.Exec(`INSERT INTO chat_archive_message_versions(conversation_id,message_key,version_hash,batch_id,position,role,body,blocks,attachments,completeness,identity_quality) VALUES(1,?,?,1,(SELECT count(*) FROM chat_archive_message_versions),'user',?,'[]','[]','source_text','source_id')`, row.key, row.hash, row.body); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db)
	found, err := repo.ListConversations(ctx, Filter{Query: "%", Title: "测试", Limit: 20})
	if err != nil || len(found) != 1 {
		t.Fatalf("search: %v %v", found, err)
	}
	none, err := repo.ListConversations(ctx, Filter{Query: "_", Limit: 20})
	if err != nil || len(none) != 0 {
		t.Fatalf("wildcard escaped: %v %v", none, err)
	}
	first, err := repo.ListMessages(ctx, 1, 0, 1)
	if err != nil || len(first) != 1 || first[0].Body != "new" || first[0].ID != 1 {
		t.Fatalf("first=%v err=%v", first, err)
	}
	second, err := repo.ListMessages(ctx, 1, first[0].ID, 1)
	if err != nil || len(second) != 1 || second[0].MessageKey != "id:b" {
		t.Fatalf("second=%v err=%v", second, err)
	}
	versions, err := repo.ListVersions(ctx, 1, "id:a", 0, 100)
	if err != nil || len(versions) != 2 || versions[0].Body != "old" {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
	var output bytes.Buffer
	if err := repo.Export(ctx, 1, "raw", &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatal(output.String())
	}
	var envelope struct {
		Conversation struct {
			AccountScope string `json:"account_scope"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &envelope); err != nil || envelope.Conversation.AccountScope != "account" {
		t.Fatalf("export lost account scope: %s", lines[0])
	}
	var batch struct {
		Record struct {
			Raw string `json:"raw_base64"`
			SHA string `json:"sha256"`
		} `json:"record"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &batch); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(batch.Record.Raw)
	if err != nil || !bytes.Equal(decoded, raw) || batch.Record.SHA != hex.EncodeToString(sum[:]) {
		t.Fatal("raw export does not round-trip")
	}
	output.Reset()
	if err := repo.Export(ctx, 1, "jsonl", &output); err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(output.String()), "\n")) != 5 {
		t.Fatal("export lost message observations")
	}
	if _, err := db.Exec(`INSERT INTO chat_archive_conversations(source,account_scope,source_id,title) VALUES('codex','second-account','session','another')`); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"jsonl", "raw"} {
		output.Reset()
		if err := repo.Export(ctx, 2, format, &output); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(bytes.Split(output.Bytes(), []byte("\n"))[0], &envelope); err != nil || envelope.Conversation.AccountScope != "second-account" {
			t.Fatalf("account identity lost for %s", format)
		}
	}
	t.Run("flat audit uses latest identity and safe source dates", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE chat_archive_message_versions SET source_time=CASE WHEN id=3 THEN '2026-01-01T09:00:00+08:00' ELSE '2026-02-30T00:00:00Z' END`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO chat_archive_sync_states(source_key,last_error,last_failure_at,last_success_at) VALUES ('failed','error',now(),NULL),('recovered','old error','2025-01-01','2026-01-01')`); err != nil {
			t.Fatal(err)
		}
		first, err := repo.ListAudit(ctx, AuditFilter{Limit: 1, Order: "asc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Messages) != 1 || first.Messages[0].Body != "new" || first.Messages[0].DeviceID != "device" || first.Messages[0].SourceID != "session" || *first.Messages[0].SourceTime != "2026-01-01T01:00:00Z" || first.Counts.Messages != 2 || first.Counts.Conversations != 2 || first.Counts.FailedSyncStates != 1 || !first.HasMore {
			t.Fatalf("%+v", first)
		}
		if _, err := db.Exec(`INSERT INTO chat_archive_message_versions(conversation_id,message_key,version_hash,batch_id,position,role,body,blocks,attachments,completeness,identity_quality,source_time) VALUES(1,'id:b','v4',1,4,'user','later observation','[]','[]','source_text','source_id','2025-01-01T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		second, err := repo.ListAudit(ctx, AuditFilter{Limit: 1, Offset: 1, Order: "asc", Snapshot: first.Snapshot})
		if err != nil || len(second.Messages) != 1 || second.Messages[0].Body != "100% literal" || second.Messages[0].SourceTime != nil || second.HasMore {
			t.Fatalf("snapshot: %+v %v", second, err)
		}
		stats, err := repo.Statistics(ctx, first.Snapshot, "2026-01-02")
		if err != nil || stats.Totals.User != 2 || stats.Unknown.User != 1 || len(stats.Days) != 7 || stats.Days[0].Date != "2026-01-01" || stats.Days[0].User != 1 {
			t.Fatalf("statistics: %+v %v", stats, err)
		}
		if _, err := db.Exec(`UPDATE chat_archive_message_versions SET source_time='2026-01-01T16:01:00Z' WHERE id=3`); err != nil {
			t.Fatal(err)
		}
		beijing, err := repo.Statistics(ctx, first.Snapshot, "2026-01-03")
		if err != nil || beijing.Days[0].Date != "2026-01-02" || beijing.Days[0].User != 1 {
			t.Fatalf("Beijing boundary: %+v %v", beijing, err)
		}
		db.Exec(`UPDATE chat_archive_message_versions SET source_time='2026-01-01T09:00:00+08:00' WHERE id=3`)
		filtered, err := repo.ListAudit(ctx, AuditFilter{Source: "codex", ConversationID: "session", MessageID: "id:a", From: "2026-01-01", To: "2026-01-01", Limit: 20})
		if err != nil || len(filtered.Messages) != 1 {
			t.Fatalf("filters: %+v %v", filtered, err)
		}
		old, err := repo.ListAudit(ctx, AuditFilter{Query: "old", Limit: 20})
		if err != nil || len(old.Messages) != 0 {
			t.Fatalf("old body matched: %+v %v", old, err)
		}
		literal, err := repo.ListAudit(ctx, AuditFilter{Query: "%", Snapshot: first.Snapshot, Limit: 20})
		if err != nil || len(literal.Messages) != 1 || literal.Messages[0].Body != "100% literal" {
			t.Fatalf("literal: %+v %v", literal, err)
		}
	})
}
