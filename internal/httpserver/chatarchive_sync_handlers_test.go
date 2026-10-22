package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/chatarchive"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"time"
)

type reconcileStub struct{ syncStub }

func (s reconcileStub) Check(context.Context, chatarchive.CheckRequest) (chatarchive.CheckResult, error) {
	return chatarchive.CheckResult{Statuses: []string{"present", "different", "missing"}}, s.err
}
func (s reconcileStub) Repair(context.Context, chatarchive.RepairRequest) (chatarchive.RepairResult, error) {
	return chatarchive.RepairResult{Inserted: 1, Verified: 1}, s.err
}

func TestSyncReconcileProtocol(t *testing.T) {
	key := strings.Repeat("k", 32)
	m := chatarchive.IngestMessage{MessageKey: "id:one", IdentityQuality: "source_id", Role: "user", Body: "正文", Blocks: []any{}, Attachments: []any{}, Completeness: "source_text", Position: 0}
	repair := chatarchive.RepairRequest{SourceKey: "0ea1449104bbc66abd94cb644f04a3a4c7dd1a81eb3ed4bae105832a038f7820", DeviceID: "设备😀", AccountScope: "账户", Source: "codex", SourcePath: "/tmp/聊天.jsonl.zst", ConversationID: "session", Messages: []chatarchive.IngestMessage{m}}
	check := chatarchive.CheckRequest{Source: "codex", AccountScope: "账户", ConversationID: "session", Messages: []chatarchive.CheckMessage{{MessageKey: "id:one", VersionHash: strings.Repeat("a", 64)}, {MessageKey: "id:two", VersionHash: strings.Repeat("b", 64)}, {MessageKey: "id:three", VersionHash: strings.Repeat("c", 64)}}}
	for _, tc := range []struct {
		path  string
		input any
		want  string
	}{{"check", check, `{"statuses":["present","different","missing"]}`}, {"repair", repair, `{"inserted":1,"verified":1}`}} {
		raw, _ := json.Marshal(tc.input)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		bodies := []string{string(raw), `{}`, strings.Repeat(" ", archiveSyncMaxBody) + "{}"}
		for name := range fields {
			var copyFields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &copyFields)
			delete(copyFields, name)
			b, _ := json.Marshal(copyFields)
			bodies = append(bodies, string(b))
		}
		bodies = append(bodies, strings.Replace(string(raw), `"source":"codex"`, `"source":"bad"`, 1))
		if tc.path == "check" {
			bodies = append(bodies, strings.Replace(string(raw), strings.Repeat("a", 64), strings.Repeat("A", 64), 1))
		} else {
			bodies = append(bodies,
				strings.Replace(string(raw), `"position":0`, `"position":-1`, 1),
				strings.Replace(string(raw), `"position":0`, `"position":null`, 1),
				strings.Replace(string(raw), `"parent_key":null,`, "", 1),
				strings.Replace(string(raw), `"blocks":[]`, `"blocks":["forbidden"]`, 1),
				strings.Replace(string(raw), repair.SourceKey, strings.Repeat("a", 64), 1))
		}
		var oversized map[string]json.RawMessage
		_ = json.Unmarshal(raw, &oversized)
		var messages []json.RawMessage
		_ = json.Unmarshal(oversized["messages"], &messages)
		many := make([]json.RawMessage, 501)
		for i := range many {
			many[i] = messages[0]
		}
		oversized["messages"], _ = json.Marshal(many)
		tooMany, _ := json.Marshal(oversized)
		bodies = append(bodies, string(tooMany))
		for i, body := range bodies {
			router := NewRouter(Dependencies{ChatArchiveSync: reconcileStub{}, ChatArchiveSyncKey: key})
			for _, authorized := range []bool{false, true} {
				req := httptest.NewRequest("POST", "/api/v1/chat-archive-sync/"+tc.path, strings.NewReader(body))
				if authorized {
					req.Header.Set("Authorization", "Bearer "+key)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				want := 400
				if !authorized {
					want = 401
				} else if i == 0 {
					want = 200
				} else if i == 2 {
					want = 413
				}
				if w.Code != want {
					t.Fatalf("%s body %d auth %v: %d %s", tc.path, i, authorized, w.Code, w.Body.String())
				}
				if want == 200 && strings.TrimSpace(w.Body.String()) != tc.want {
					t.Fatal(w.Body.String())
				}
			}
		}
	}
}

func TestSyncReconcileOptionalServiceAndErrors(t *testing.T) {
	key := strings.Repeat("k", 32)
	for _, tc := range []struct {
		service archiveSyncService
		want    int
	}{
		{syncStub{}, 503},
		{reconcileStub{syncStub{err: chatarchive.ErrIngestConflict}}, 409},
		{reconcileStub{syncStub{err: errors.New("private-body-and-key")}}, 500},
	} {
		router := NewRouter(Dependencies{ChatArchiveSync: tc.service, ChatArchiveSyncKey: key})
		req := httptest.NewRequest("POST", "/api/v1/chat-archive-sync/check", strings.NewReader(`{"source":"codex","account_scope":"account","conversation_id":"session","messages":[]}`))
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want || strings.Contains(w.Body.String(), "private-body") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestSyncRejectionLogsSafeReason(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	w := httptest.NewRecorder()
	syncError(w, chatarchive.ErrInvalidIngest)
	if !strings.Contains(logs.String(), "chat_archive_sync_rejected") {
		t.Fatal("rejection not observable")
	}
}

func TestSyncStructureErrorLocatesFieldWithoutValues(t *testing.T) {
	key := strings.Repeat("k", 32)
	router := NewRouter(Dependencies{ChatArchiveSync: syncStub{}, ChatArchiveSyncKey: key})
	for _, tc := range []struct{ body, want string }{
		{`{}`, "json.structure.$.source_key.missing"},
		{`{"source_key":null}`, "json.structure.$.source_key.type"},
		{`{"source_key":"` + strings.Repeat("a", 64) + `","PRIVATE_SECRET_FIELD":"PRIVATE_SECRET_VALUE"}`, "json.structure.$.extra_fields"},
	} {
		r := httptest.NewRequest("POST", "/api/v1/chat-archive-sync/state", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 400 || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "PRIVATE_SECRET") {
			t.Fatalf("bad diagnostic: %d %s", w.Code, w.Body.String())
		}
	}
}

type syncStub struct{ err error }

func (s syncStub) LegacyCounts(context.Context) (chatarchive.LegacyCounts, error) {
	return chatarchive.LegacyCounts{Batches: 2, Messages: 3}, s.err
}
func (s syncStub) SyncState(context.Context, string) (*chatarchive.SyncState, error) {
	return nil, s.err
}
func (s syncStub) Upload(context.Context, chatarchive.UploadRequest) error { return s.err }
func (s syncStub) VerifyIngest(context.Context) (chatarchive.VerifyResult, error) {
	return chatarchive.VerifyResult{Verified: true, Batches: 4, Messages: 5}, s.err
}

func TestSyncAuthenticationAndProtocol(t *testing.T) {
	key := strings.Repeat("k", 32)
	for _, tc := range []struct {
		name, config, header, query string
		want                        int
	}{
		{"disabled", "", "Bearer " + key, "", 401}, {"short", "short", "Bearer short", "", 401},
		{"missing", key, "", "", 401}, {"query", key, "", "?token=" + key, 401},
		{"basic", key, "Basic " + key, "", 401}, {"wrong", key, "Bearer " + strings.Repeat("z", 32), "", 401},
		{"valid", key, "Bearer " + key, "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Dependencies{ChatArchiveSync: syncStub{}, ChatArchiveSyncKey: tc.config})
			req := httptest.NewRequest("GET", "/api/v1/chat-archive-sync/status"+tc.query, nil)
			req.Header.Set("Authorization", tc.header)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d", w.Code)
			}
			if w.Code == 200 && strings.TrimSpace(w.Body.String()) != `{"legacy_batches":2,"legacy_messages":3}` {
				t.Fatal(w.Body.String())
			}
		})
	}
	router := NewRouter(Dependencies{ChatArchiveSync: syncStub{}, ChatArchiveSyncKey: key, ChatArchive: archiveStub{}, Auth: auth.NewService(auth.NewRepository(nil), auth.NewTokenManager("admin-secret", time.Hour)), RBAC: rbac.NewService(rbac.NewRepository(nil))})
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "state", `{"source_key":"` + strings.Repeat("a", 64) + `"}`, 200},
		{"POST", "state", `{"source_key":"bad"}`, 400},
		{"POST", "state", `{"source_key":"` + strings.Repeat("a", 64) + `","unknown":1}`, 400},
		{"POST", "state", `{"source_key":"` + strings.Repeat("a", 64) + `"} {}`, 400},
		{"POST", "state", `{"source_key":"` + strings.Repeat("a", 64) + `","source_key":"` + strings.Repeat("b", 64) + `"}`, 400},
		{"POST", "upload", `{}`, 400},
		{"POST", "upload", strings.Repeat(" ", 16<<20) + "{}", 413},
		{"GET", "verify", "", 200},
	} {
		req := httptest.NewRequest(tc.method, "/api/v1/chat-archive-sync/"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest("GET", "/api/v1/chat-archive/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("sync key granted admin", w.Code)
	}
}

func TestSyncErrorsAreGeneric(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{chatarchive.ErrIngestConflict, 409}, {chatarchive.ErrInvalidIngest, 400}, {errors.New("private-body-and-key"), 500}} {
		key := strings.Repeat("k", 32)
		router := NewRouter(Dependencies{ChatArchiveSync: syncStub{err: tc.err}, ChatArchiveSyncKey: key})
		req := httptest.NewRequest("GET", "/api/v1/chat-archive-sync/status", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want || strings.Contains(w.Body.String(), "private-body") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

var _ http.Handler = NewRouter(Dependencies{})

type blockingSyncStub struct {
	syncStub
	entered chan struct{}
	release chan struct{}
}

func (s blockingSyncStub) LegacyCounts(ctx context.Context) (chatarchive.LegacyCounts, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
		return chatarchive.LegacyCounts{}, nil
	case <-ctx.Done():
		return chatarchive.LegacyCounts{}, ctx.Err()
	}
}

func TestSyncConcurrencyAndRateLimits(t *testing.T) {
	key := strings.Repeat("k", 32)
	request := func(handler http.Handler) int {
		r := httptest.NewRequest("GET", "/api/v1/chat-archive-sync/status", nil)
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	stub := blockingSyncStub{entered: make(chan struct{}, 2), release: make(chan struct{})}
	router := NewRouter(Dependencies{ChatArchiveSync: stub, ChatArchiveSyncKey: key})
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if got := request(router); got != 200 {
				t.Errorf("admitted request %d", got)
			}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-stub.entered:
		case <-time.After(5 * time.Second):
			close(stub.release)
			workers.Wait()
			t.Fatal("concurrent requests did not enter")
		}
	}
	if got := request(router); got != 429 {
		t.Errorf("concurrency overflow: %d", got)
	}
	close(stub.release)
	workers.Wait()
	router = NewRouter(Dependencies{ChatArchiveSync: syncStub{}, ChatArchiveSyncKey: key})
	limited := false
	for i := 0; i < 150; i++ {
		if request(router) == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("burst not limited")
	}
}

func TestSyncUploadStrictShapeAndUnicode(t *testing.T) {
	const valid = `{"source_key":"0ea1449104bbc66abd94cb644f04a3a4c7dd1a81eb3ed4bae105832a038f7820","device_id":"设备😀","account_scope":"账户","source":"codex","source_path":"/tmp/聊天.jsonl.zst","expected_state":null,"state":{"offset":100,"prefix_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","conversation_id":"session","parser_version":"dialogue-only-3"},"batches":[{"start_offset":0,"conversation_id":"session","unknown_events":0,"messages":[{"message_key":"id:你好😀","parent_key":null,"identity_quality":"source_id","role":"user","body":"正文","blocks":[],"attachments":[],"completeness":"source_text","source_time":null,"position":12}]}]}`
	key := strings.Repeat("k", 32)
	router := NewRouter(Dependencies{ChatArchiveSync: syncStub{}, ChatArchiveSyncKey: key})
	for name, body := range map[string]string{
		"valid":                           valid,
		"state metadata":                  strings.Replace(valid, `"state":{`, `"state":{"client_metadata":{"note":"not archived"},`, 1),
		"state metadata missing required": strings.Replace(strings.Replace(valid, `"state":{`, `"state":{"client_metadata":true,`, 1), `"offset":100,`, "", 1),
		"state metadata wrong version":    strings.Replace(strings.Replace(valid, `"state":{`, `"state":{"client_metadata":true,`, 1), `dialogue-only-3`, `unsupported`, 1),
		"surrogate pair":                  strings.Replace(valid, "正文", `\ud83d\ude00`, 1),
		"missing CAS":                     strings.Replace(valid, `"expected_state":null,`, "", 1),
		"missing offset":                  strings.Replace(valid, `"start_offset":0,`, "", 1),
		"null offset":                     strings.Replace(valid, `"start_offset":0`, `"start_offset":null`, 1),
		"unknown message":                 strings.Replace(valid, `"role":"user"`, `"role":"user","raw":"secret"`, 1),
		"duplicate message":               strings.Replace(valid, `"role":"user"`, `"role":"user","role":"assistant"`, 1),
		"missing nullable":                strings.Replace(valid, `"source_time":null,`, "", 1),
		"null blocks":                     strings.Replace(valid, `"blocks":[]`, `"blocks":null`, 1),
		"isolated surrogate":              strings.Replace(valid, "正文", `\ud800`, 1),
		"invalid UTF8":                    strings.Replace(valid, "正文", string([]byte{0xff}), 1),
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/chat-archive-sync/upload", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+key)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			want := 400
			if name == "valid" || name == "surrogate pair" || name == "state metadata" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
