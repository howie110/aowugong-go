package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/blog"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

func blogTestRouter(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	db := testdatabase.Open(t)
	roles := rbac.NewService(rbac.NewRepository(db))
	if err := roles.SyncDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Dependencies{Auth: auth.NewService(auth.NewRepository(db), auth.NewTokenManager("blog-test", time.Hour)), RBAC: roles, BlogStatuses: blog.NewStatusService(blog.NewRepository(db))})
	createHTTPTestUser(t, db, "blog-admin", "admin@example.test", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "blog-viewer", "viewer@example.test", "password", rbac.InvestorRoleCode)
	return h, loginHTTPTestUser(t, h, "blog-admin", "password"), loginHTTPTestUser(t, h, "blog-viewer", "password")
}
func TestStatusWriteRequiresPermission(t *testing.T) {
	h, admin, viewer := blogTestRouter(t)
	body := `{"id":"11111111-1111-4111-8111-111111111111","body":"private draft","images":[],"published":false}`
	for _, tc := range []struct {
		token string
		want  int
	}{{"", 401}, {viewer, 403}, {admin, 201}} {
		req := httptest.NewRequest("POST", "/api/v1/blog/admin/statuses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("got %d want %d: %s", rec.Code, tc.want, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/blog/statuses", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private draft") {
		t.Fatalf("draft leak: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/blog/statuses?limit=nope", nil))
	if rec.Code != 400 {
		t.Fatalf("invalid limit %d", rec.Code)
	}
}
