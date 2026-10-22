package chatarchive

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

func repairFixture() RepairRequest {
	u := ingestFixture()
	return RepairRequest{SourceKey: u.SourceKey, DeviceID: u.DeviceID, AccountScope: u.AccountScope, Source: u.Source, SourcePath: u.SourcePath, ConversationID: u.State.ConversationID, Messages: u.Batches[0].Messages}
}

func TestPostgresReconcileConcurrentAcrossDevices(t *testing.T) {
	db := ingestTestDB(t)
	dsn := os.Getenv("CHAT_ARCHIVE_TEST_DSN")
	if strings.Contains(dsn, "://") {
		p, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := p.Query()
		q.Set("options", "-c search_path=chat_archive_ingest_test")
		p.RawQuery = q.Encode()
		dsn = p.String()
	} else {
		dsn += " options='-c search_path=chat_archive_ingest_test'"
	}
	concurrent, err := database.OpenPostgres(context.Background(), config.Database{URL: dsn, MaxOpenConns: 4, MaxIdleConns: 4, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer concurrent.Close()
	repo := NewRepository(concurrent)
	start := make(chan struct{})
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func(i int) {
			r := repairFixture()
			r.DeviceID = fmt.Sprintf("device-%d", i)
			r.SourceKey = sourceKey(UploadRequest{DeviceID: r.DeviceID, AccountScope: r.AccountScope, Source: r.Source, SourcePath: r.SourcePath})
			<-start
			_, err := repo.Repair(context.Background(), r)
			results <- err
		}(i)
	}
	close(start)
	for i := 0; i < 4; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	v, err := repo.VerifyIngest(context.Background())
	if err != nil || v.Messages != 1 || v.Batches != 1 {
		t.Fatalf("concurrent duplicates %+v %v", v, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM chat_archive_sync_states`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("new cursor %d %v", count, err)
	}
	// An ordinary Upload holds this same source lock. Repair must wait before reading.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	lockKey, _ := hex.DecodeString(ingestFixture().SourceKey)
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(binary.BigEndian.Uint64(lockKey[:8]))); err != nil {
		t.Fatal(err)
	}
	// Use a fresh source with the fixed UTF-8 fixture key's first 64 bits.
	done := make(chan error, 1)
	go func() { _, err := repo.Repair(context.Background(), repairFixture()); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("repair bypassed source lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Concurrent ordinary upload and repeated repairs share the source lock;
	// the repair must neither overwrite the upload's cursor nor duplicate itself.
	u := ingestFixture()
	u.Batches[0].Messages[0].MessageKey = "id:ordinary-upload"
	r := repairFixture()
	r.Messages[0].MessageKey = "id:concurrent-repair"
	start = make(chan struct{})
	results = make(chan error, 5)
	go func() { <-start; results <- repo.Upload(context.Background(), u) }()
	for i := 0; i < 4; i++ {
		go func() { <-start; _, err := repo.Repair(context.Background(), r); results <- err }()
	}
	close(start)
	for i := 0; i < 5; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	v, err = repo.VerifyIngest(context.Background())
	if err != nil || v.Messages != 3 || v.Batches != 3 {
		t.Fatalf("upload and repair %+v %v", v, err)
	}
	state, err := repo.SyncState(context.Background(), u.SourceKey)
	if err != nil || !reflect.DeepEqual(state, &u.State) {
		t.Fatalf("upload cursor changed %+v %v", state, err)
	}
}

func TestReconcileValidation(t *testing.T) {
	for name, mutate := range map[string]func(*RepairRequest){
		"source":       func(r *RepairRequest) { r.Source = "other" },
		"account":      func(r *RepairRequest) { r.AccountScope = "" },
		"device":       func(r *RepairRequest) { r.DeviceID = "" },
		"path":         func(r *RepairRequest) { r.SourcePath = "relative" },
		"hash":         func(r *RepairRequest) { r.SourceKey = strings.Repeat("a", 64) },
		"conversation": func(r *RepairRequest) { r.ConversationID = "" },
		"message":      func(r *RepairRequest) { r.Messages[0].Role = "tool" },
		"negative":     func(r *RepairRequest) { r.Messages[0].Position = -1 },
		"unordered":    func(r *RepairRequest) { r.Messages = append(r.Messages, r.Messages[0]) },
		"nil":          func(r *RepairRequest) { r.Messages = nil },
		"limit":        func(r *RepairRequest) { r.Messages = make([]IngestMessage, 501) },
	} {
		t.Run(name, func(t *testing.T) {
			r := repairFixture()
			mutate(&r)
			if !errors.Is(r.Validate(), ErrInvalidIngest) {
				t.Fatal("accepted invalid repair")
			}
		})
	}
	if err := repairFixture().Validate(); err != nil {
		t.Fatal(err)
	}
	c := CheckRequest{Source: "codex", AccountScope: "account", ConversationID: "session", Messages: []CheckMessage{{MessageKey: "id:a", VersionHash: strings.Repeat("a", 64)}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CheckRequest){
		func(c *CheckRequest) { c.Source = "bad" }, func(c *CheckRequest) { c.AccountScope = "" }, func(c *CheckRequest) { c.ConversationID = "" }, func(c *CheckRequest) { c.Messages = nil }, func(c *CheckRequest) { c.Messages = make([]CheckMessage, 501) }, func(c *CheckRequest) {
			c.Messages = []CheckMessage{{MessageKey: "", VersionHash: strings.Repeat("a", 64)}}
		}, func(c *CheckRequest) {
			c.Messages = []CheckMessage{{MessageKey: "id:a", VersionHash: strings.Repeat("A", 64)}}
		},
	} {
		bad := c
		mutate(&bad)
		if !errors.Is(bad.Validate(), ErrInvalidIngest) {
			t.Fatal("accepted invalid check")
		}
	}
}

func TestPostgresReconcileVersionsStateAndRollback(t *testing.T) {
	db := ingestTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	u := ingestFixture()
	if err := repo.Upload(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE chat_archive_sync_states SET state=state || '{"native_user_backfill_v1":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],"custom":"keep"}'::jsonb,last_error='keep',last_failure_at=now()`); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var s string
		if err := db.QueryRow(`SELECT row_to_json(s)::text FROM chat_archive_sync_states s`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	r := repairFixture()
	old := r.Messages[0]
	r.Messages[0].Body = "历史版本😀\n<&>\u2028"
	changed := r.Messages[0]
	repeated := changed
	repeated.Position++
	distinct := changed
	distinct.Position += 2
	distinct.MessageKey = "id:distinct"
	r.Messages = append(r.Messages, repeated, distinct)
	for i := 0; i < 2; i++ {
		got, err := repo.Repair(ctx, r)
		want := int64(2)
		if i > 0 {
			want = 0
		}
		if err != nil || got.Inserted != want || got.Verified != 3 {
			t.Fatalf("repair %+v %v", got, err)
		}
	}
	c := CheckRequest{Source: r.Source, AccountScope: r.AccountScope, ConversationID: r.ConversationID, Messages: []CheckMessage{{old.MessageKey, messageHash(old)}, {changed.MessageKey, messageHash(changed)}, {old.MessageKey, strings.Repeat("a", 64)}, {"id:absent", messageHash(old)}}}
	got, err := repo.Check(ctx, c)
	if err != nil || !reflect.DeepEqual(got.Statuses, []string{"present", "present", "different", "missing"}) {
		t.Fatalf("check %+v %v", got, err)
	}
	c.AccountScope = "another"
	got, err = repo.Check(ctx, c)
	if err != nil || !reflect.DeepEqual(got.Statuses, []string{"missing", "missing", "missing", "missing"}) {
		t.Fatalf("scope %+v %v", got, err)
	}
	if snapshot() != before {
		t.Fatal("sync state mutated")
	}
	v, err := repo.VerifyIngest(ctx)
	if err != nil || !v.Verified || v.Messages != 3 || v.Batches != 2 {
		t.Fatalf("verify %+v %v", v, err)
	}
	bad := r
	bad.ConversationID = "another"
	if _, err := repo.Repair(ctx, bad); !errors.Is(err, ErrIngestConflict) {
		t.Fatal("source rebound", err)
	}
	if _, err := db.Exec(`ALTER TABLE chat_archive_message_versions ADD CONSTRAINT reject_repair CHECK(body<>'reject-repair')`); err != nil {
		t.Fatal(err)
	}
	r.Messages = []IngestMessage{old, distinct}
	r.Messages[0].Body = "new-before-failure"
	r.Messages[1].Body = "reject-repair"
	if _, err := repo.Repair(ctx, r); err == nil {
		t.Fatal("expected rollback")
	}
	after, err := repo.VerifyIngest(ctx)
	if err != nil || after != v || snapshot() != before {
		t.Fatalf("rollback %+v %v", after, err)
	}
}

func TestPostgresUploadRetryAfterRepair(t *testing.T) {
	db := ingestTestDB(t)
	repo, ctx := NewRepository(db), context.Background()
	u := ingestFixture()
	if err := repo.Upload(ctx, u); err != nil {
		t.Fatal(err)
	}
	r := repairFixture()
	r.Messages[0].MessageKey = "id:recovered"
	if _, err := repo.Repair(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upload(ctx, u); err != nil {
		t.Fatalf("repair invalidated committed upload retry: %v", err)
	}
	v, err := repo.VerifyIngest(ctx)
	if err != nil || v.Messages != 2 {
		t.Fatalf("retry duplicated records: %+v %v", v, err)
	}
}

func TestPostgresReconcileUninitializedStateAndLimits(t *testing.T) {
	db := ingestTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	r := repairFixture()
	if _, err := db.Exec(`INSERT INTO chat_archive_sync_states(source_key,last_error) VALUES(?,'failed-before-first-upload')`, r.SourceKey); err != nil {
		t.Fatal(err)
	}
	m := r.Messages[0]
	r.Messages = make([]IngestMessage, 500)
	for i := range r.Messages {
		r.Messages[i] = m
		r.Messages[i].Position = int64(i)
		r.Messages[i].MessageKey = fmt.Sprintf("id:%d", i)
	}
	got, err := repo.Repair(ctx, r)
	if err != nil || got.Inserted != 500 || got.Verified != 500 {
		t.Fatalf("500 repair %+v %v", got, err)
	}
	var unchanged bool
	if err := db.QueryRow(`SELECT state IS NULL AND last_success_at IS NULL AND last_error='failed-before-first-upload' FROM chat_archive_sync_states`).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("initial state changed %v", err)
	}
	c := CheckRequest{Source: r.Source, AccountScope: r.AccountScope, ConversationID: r.ConversationID, Messages: []CheckMessage{}}
	checked, err := repo.Check(ctx, c)
	if err != nil || checked.Statuses == nil || len(checked.Statuses) != 0 {
		t.Fatalf("empty check %+v %v", checked, err)
	}
	r.Messages = []IngestMessage{}
	got, err = repo.Repair(ctx, r)
	if err != nil || got != (RepairResult{}) {
		t.Fatalf("empty repair %+v %v", got, err)
	}
	v, err := repo.VerifyIngest(ctx)
	if err != nil || !v.Verified || v.Messages != 500 || v.Batches != 1 {
		t.Fatalf("verify %+v %v", v, err)
	}
}
