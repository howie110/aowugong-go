package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/chatarchive"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type archiveStub struct{}
type brokenArchiveExport struct{ archiveStub }

func (archiveStub) Statistics(context.Context, int64, string) (chatarchive.Statistics, error) {
	return chatarchive.Statistics{}, nil
}
func (archiveStub) ListAudit(_ context.Context, f chatarchive.AuditFilter) (chatarchive.AuditPage, error) {
	return chatarchive.AuditPage{Messages: []chatarchive.AuditMessage{{MessageKey: f.MessageID, SourceID: f.ConversationID}}, Snapshot: f.Snapshot}, nil
}

func (brokenArchiveExport) Export(_ context.Context, _ int64, _ string, w io.Writer) error {
	_, _ = io.WriteString(w, "{\"kind\":\"conversation\"}\n")
	return errors.New("synthetic query failure")
}

func (archiveStub) ListConversations(context.Context, chatarchive.Filter) ([]chatarchive.Conversation, error) {
	return []chatarchive.Conversation{{ID: 1, Title: "synthetic"}}, nil
}
func (archiveStub) ListMessages(_ context.Context, id, after int64, limit int) ([]chatarchive.Message, error) {
	if id == 404 {
		return nil, sql.ErrNoRows
	}
	return []chatarchive.Message{}, nil
}
func (archiveStub) ListVersions(context.Context, int64, string, int64, int) ([]chatarchive.Message, error) {
	return []chatarchive.Message{}, nil
}
func (archiveStub) Export(_ context.Context, _ int64, _ string, w io.Writer) error {
	_, err := io.WriteString(w, "{\"kind\":\"export_complete\"}\n")
	return err
}

func TestArchiveRoutesEnforceAdminAndValidate(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := context.Background()
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("archive-test-secret", time.Hour))
	router := NewRouter(Dependencies{Auth: authService, RBAC: rbacService, ChatArchive: archiveStub{}})
	createHTTPTestUser(t, db, "archive-admin", "a@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "archive-user", "u@example.com", "password", rbac.InvestorRoleCode)
	admin := loginHTTPTestUser(t, router, "archive-admin", "password")
	user := loginHTTPTestUser(t, router, "archive-user", "password")
	// 即便误授页面权限，普通角色也不能读取归档。
	_, err := db.Exec(`INSERT INTO aowugong_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM aowugong_roles r,aowugong_permissions p WHERE r.code=? AND p.code=?`, rbac.InvestorRoleCode, rbac.PermissionChatArchive)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/chat-archive/conversations"
	for _, tc := range []struct {
		path, token string
		status      int
	}{

		{"/api/v1/chat-archive/messages", "", 401},
		{"/api/v1/chat-archive/messages", user, 403},
		{"/api/v1/chat-archive/messages?conversation_id=session&message_id=id:m&snapshot=9&order=asc", admin, 200},
		{"/api/v1/chat-archive/messages?order=invalid", admin, 400},
		{"/api/v1/chat-archive/messages?from=2026-02-30", admin, 400},
		{"/api/v1/chat-archive/messages?snapshot=-1", admin, 400},
		{"/api/v1/chat-archive/messages?offset=1000001", admin, 400},
		{base, "", 401}, {base, user, 403}, {base, admin, 200}, {base + "?limit=0", admin, 400}, {base + "?from=bad", admin, 400},
		{base + "/404/messages", admin, 404}, {base + "/1/messages?after=-1", admin, 400}, {base + "/1/versions", admin, 400},
		{base + "/1/export?format=raw", admin, 200}, {base + "/1/export?format=html", admin, 400},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("%s got %d want %d: %s", tc.path, res.Code, tc.status, res.Body.String())
		}
		if tc.status == 200 && res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response cache enabled")
		}
		if strings.Contains(tc.path, "conversation_id=session") && (!strings.Contains(res.Body.String(), `"source_id":"session"`) || !strings.Contains(res.Body.String(), `"message_key":"id:m"`) || !strings.Contains(res.Body.String(), `"snapshot":9`)) {
			t.Fatal(res.Body.String())
		}
	}
	t.Run("partial export aborts instead of reporting success", func(t *testing.T) {
		broken := NewRouter(Dependencies{Auth: authService, RBAC: rbacService, ChatArchive: brokenArchiveExport{}})
		req := httptest.NewRequest(http.MethodGet, base+"/1/export?format=raw", nil)
		req.Header.Set("Authorization", "Bearer "+admin)
		response := httptest.NewRecorder()
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Error("partial export did not abort")
			}
			if strings.Contains(response.Body.String(), "export_complete") {
				t.Error("partial export marked complete")
			}
		}()
		broken.ServeHTTP(response, req)
	})
}
