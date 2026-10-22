package chatarchive

import (
	"context"
	"database/sql"
	"errors"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func ingestTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CHAT_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN not supplied")
	}
	db, err := database.OpenPostgres(context.Background(), config.Database{URL: dsn, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var name string
	if err = db.QueryRow("SELECT current_database()").Scan(&name); err != nil || name != "chat_archive_test" {
		t.Fatal("refusing non-test database")
	}
	if _, err = db.Exec("CREATE SCHEMA chat_archive_ingest_test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA chat_archive_ingest_test CASCADE") })
	if _, err = db.Exec("SET search_path=chat_archive_ingest_test"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/postgres/00013_chat_archive.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	migration, err = os.ReadFile("../../migrations/postgres/00014_chat_archive_time.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPostgresIngestAtomicCASRetryAndVerify(t *testing.T) {
	db := ingestTestDB(t)
	r := NewRepository(db)
	ctx := context.Background()
	u := ingestFixture()
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("retry", err)
	}
	result, err := r.VerifyIngest(ctx)
	if err != nil || !result.Verified || result.Batches != 1 || result.Messages != 1 {
		t.Fatalf("verify %+v %v", result, err)
	}
	u.Batches[0].Messages[0].Body = "different"
	if err := r.Upload(ctx, u); !errors.Is(err, ErrIngestConflict) {
		t.Fatal("different retry", err)
	}
	u = ingestFixture()
	u.State.Offset = 200
	u.Batches[0].StartOffset = 100
	u.Batches[0].Messages[0].Position = 120
	previous := ingestFixture().State
	u.ExpectedState = &previous
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("append", err)
	}
	u.State.Offset = 300
	if err := r.Upload(ctx, u); !errors.Is(err, ErrIngestConflict) {
		t.Fatal("stale CAS", err)
	}
	state, err := r.SyncState(ctx, u.SourceKey)
	if err != nil || state.Offset != 200 {
		t.Fatal("state", state, err)
	}
	// Force a failure after conversation/batch writes to prove rollback of the whole upload.
	if _, err := db.Exec(`ALTER TABLE chat_archive_message_versions ADD CONSTRAINT reject_test_body CHECK(body<>'reject-test')`); err != nil {
		t.Fatal(err)
	}
	u.ExpectedState = state
	u.Batches[0].StartOffset = 200
	u.Batches[0].Messages[0].Position = 220
	u.Batches[0].Messages[0].Body = "reject-test"
	if err := r.Upload(ctx, u); err == nil {
		t.Fatal("expected database failure")
	}
	result, err = r.VerifyIngest(ctx)
	if err != nil || result.Batches != 2 {
		t.Fatalf("rollback %+v %v", result, err)
	}
	if _, err := db.Exec(`UPDATE chat_archive_message_versions SET body='tampered' WHERE position=12`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.VerifyIngest(ctx); !errors.Is(err, ErrArchiveIntegrity) {
		t.Fatal("tamper", err)
	}
}

func TestPostgresIngestConcurrentRetryAndRescan(t *testing.T) {
	db := ingestTestDB(t)
	dsn := os.Getenv("CHAT_ARCHIVE_TEST_DSN")
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := parsed.Query()
		q.Set("options", "-c search_path=chat_archive_ingest_test")
		parsed.RawQuery = q.Encode()
		dsn = parsed.String()
	} else {
		dsn += " options='-c search_path=chat_archive_ingest_test'"
	}
	concurrent, err := database.OpenPostgres(context.Background(), config.Database{URL: dsn, MaxOpenConns: 4, MaxIdleConns: 4, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer concurrent.Close()
	r := NewRepository(concurrent)
	ctx := context.Background()
	u := ingestFixture()
	start := make(chan struct{})
	results := make(chan error, 4)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; results <- r.Upload(ctx, u) }()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent exact retry", err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM chat_archive_raw_batches`).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent duplicates", count, err)
	}
	// 同一历史补传同时到达：共享 source-key 锁，只有一份提交。
	backfill := backfillFixture(t, u.State)
	backfillResults := make(chan error, 4)
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); backfillResults <- r.Upload(ctx, backfill) }()
	}
	workers.Wait()
	close(backfillResults)
	for err := range backfillResults {
		if err != nil {
			t.Fatal("concurrent backfill", err)
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM chat_archive_raw_batches`).Scan(&count); err != nil || count != 2 {
		t.Fatal("backfill duplicated", count, err)
	}
	latest, err := r.SyncState(ctx, u.SourceKey)
	if err != nil {
		t.Fatal(err)
	}
	// 后续旧的增量请求必须冲突，而不是覆盖刚写入的标记。
	stale := ingestFixture()
	stale.ExpectedState = &u.State
	stale.State.Offset = 200
	stale.Batches[0].StartOffset = 100
	stale.Batches[0].Messages[0].Position = 120
	if err := r.Upload(ctx, stale); !errors.Is(err, ErrIngestConflict) {
		t.Fatal("stale regular sync", err)
	}
	u.State = *latest
	prior := u.State
	u.ExpectedState = &prior
	u.State.Offset = 70
	u.State.PrefixSHA256 = strings.Repeat("c", 64)
	u.Batches[0].Messages[0].Body = "rewritten"
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("offset reset rescan", err)
	}
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("rescan retry", err)
	}
	// Compression changes the metadata path but preserves identity and retry semantics.
	u.SourcePath = strings.TrimSuffix(u.SourcePath, ".zst")
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("compressed retry", err)
	}
	// Empty observations are needed to advance over records filtered by the parser.
	prior = u.State
	u.ExpectedState = &prior
	u.State.Offset = 90
	u.Batches = []IngestBatch{{StartOffset: 70, ConversationID: "session", Messages: []IngestMessage{}, UnknownEvents: 1}}
	if err := r.Upload(ctx, u); err != nil {
		t.Fatal("empty batch", err)
	}
	result, err := r.VerifyIngest(ctx)
	if err != nil || result.Batches != 4 || result.Messages != 3 {
		t.Fatalf("verify %+v %v", result, err)
	}
}
