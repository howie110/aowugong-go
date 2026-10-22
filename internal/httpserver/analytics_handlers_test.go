package httpserver

import (
	"context"
	"github.com/howiedata/aowugong-go/internal/analytics"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type analyticsStub struct {
	calls int
	ip    string
}

func (s *analyticsStub) Collect(_ context.Context, _ analytics.Payload, ip, _, _ string) error {
	s.calls++
	s.ip = ip
	return nil
}
func (s *analyticsStub) Report(_ context.Context, f analytics.Filter) (analytics.Report, error) {
	return analytics.Report{From: f.From, To: f.To}, nil
}
func TestAnalyticsCollectionBoundary(t *testing.T) {
	s := &analyticsStub{}
	router := NewRouter(Dependencies{Analytics: s, AnalyticsPublicURL: "https://aowugong.top", AnalyticsTrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	for _, tc := range []struct {
		origin, body string
		status       int
	}{
		{"https://evil.test", `{}`, 403}, {"", `{}`, 403}, {"https://aowugong.top", `{"id":"x","password":"secret"}`, 400}, {"https://aowugong.top", `{} {}`, 400}, {"https://aowugong.top", `{"url":"` + strings.Repeat("a", 9000) + `"}`, 400}, {"https://aowugong.top", `{"id":"x","url":"/work"}`, 204},
	} {
		r := httptest.NewRequest(http.MethodPost, "https://aowugong.top/api/v1/analytics/collect", strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", "192.0.2.3")
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	if s.calls != 1 || s.ip != "192.0.2.3" {
		t.Fatalf("unexpected collection %+v", s)
	}
}
func TestAnalyticsProxySpoofing(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8")}
	for _, tc := range []struct{ remote, xff, want string }{{"203.0.113.1:123", "1.2.3.4", "203.0.113.1"}, {"127.0.0.1:123", "1.2.3.4, 203.0.113.9, 10.0.0.2", "203.0.113.9"}, {"127.0.0.1:123", "malformed", "127.0.0.1"}} {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", tc.xff)
		if got := analyticsClientIP(r, proxies); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}
func TestAnalyticsAdminOnly(t *testing.T) {
	db := testdatabase.Open(t)
	rb := rbac.NewService(rbac.NewRepository(db))
	if err := rb.SyncDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("analytics-test-secret", time.Hour))
	router := NewRouter(Dependencies{Auth: a, RBAC: rb, Analytics: &analyticsStub{}})
	createHTTPTestUser(t, db, "analytics-admin", "aa@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "analytics-user", "au@example.com", "password", rbac.InvestorRoleCode)
	admin := loginHTTPTestUser(t, router, "analytics-admin", "password")
	user := loginHTTPTestUser(t, router, "analytics-user", "password")
	if _, err := db.Exec(`INSERT INTO aowugong_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM aowugong_roles r,aowugong_permissions p WHERE r.code=? AND p.code=?`, rbac.InvestorRoleCode, rbac.PermissionAnalytics); err != nil {
		t.Fatal(err)
	}
	query := "?from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z"
	for _, tc := range []struct {
		token, q string
		status   int
	}{{"", query, 401}, {user, query, 403}, {admin, query, 200}, {admin, query + "&invalid=x", 400}, {admin, "?from=bad", 400}} {
		r := httptest.NewRequest("GET", "/api/v1/analytics/report/"+tc.q, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
}
