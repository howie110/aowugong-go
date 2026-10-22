package httpserver

import (
	"context"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"github.com/howiedata/aowugong-go/internal/trajectory"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type trajectoryStub struct {
	calls int
	owner int64
}

func (s *trajectoryStub) Collect(_ context.Context, token string, p trajectory.Point) (bool, error) {
	s.calls++
	if token != strings.Repeat("a", 64) {
		return false, trajectory.ErrUnauthorized
	}
	return true, nil
}
func (s *trajectoryStub) Devices(_ context.Context, owner int64) ([]trajectory.Device, error) {
	s.owner = owner
	return []trajectory.Device{}, nil
}
func (s *trajectoryStub) Create(context.Context, int64, string) (trajectory.Credential, error) {
	return trajectory.Credential{}, nil
}
func (s *trajectoryStub) Rotate(context.Context, int64, string) (trajectory.Credential, error) {
	return trajectory.Credential{}, nil
}
func (s *trajectoryStub) Revoke(context.Context, int64, string) error { return nil }
func (s *trajectoryStub) Report(context.Context, int64, trajectory.Query) (trajectory.Report, error) {
	return trajectory.Report{}, nil
}
func TestTrajectoryUploadValidation(t *testing.T) {
	s := &trajectoryStub{}
	router := NewRouter(Dependencies{Trajectory: s})
	body := "timestamp=1792540800&lat=23&lon=113&acc=5"
	for _, tc := range []struct {
		body, query, token, content string
		status                      int
	}{
		{body, "", strings.Repeat("a", 64), "application/x-www-form-urlencoded", 200},
		{body, "", "", "application/x-www-form-urlencoded", 401},
		{body, "", strings.Repeat("b", 64), "application/x-www-form-urlencoded", 401},
		{body, "?token=private", strings.Repeat("a", 64), "application/x-www-form-urlencoded", 400},
		{body + "&lat=24", "", strings.Repeat("a", 64), "application/x-www-form-urlencoded", 400},
		{body + "&secret=private", "", strings.Repeat("a", 64), "application/x-www-form-urlencoded", 400},
		{body, "", strings.Repeat("a", 64), "application/json", 415},
		{strings.Repeat("x", 9000), "", strings.Repeat("a", 64), "application/x-www-form-urlencoded", 413},
	} {
		r := httptest.NewRequest("POST", "/api/v1/trajectory/collect"+tc.query, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.content)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	if s.calls != 2 {
		t.Fatalf("malformed requests reached database %d", s.calls)
	}
}
func TestTrajectoryAdminOnly(t *testing.T) {
	db := testdatabase.Open(t)
	rb := rbac.NewService(rbac.NewRepository(db))
	if e := rb.SyncDefaults(context.Background()); e != nil {
		t.Fatal(e)
	}
	a := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("trajectory-test-secret", time.Hour))
	s := &trajectoryStub{}
	router := NewRouter(Dependencies{Auth: a, RBAC: rb, Trajectory: s})
	createHTTPTestUser(t, db, "trajectory-admin", "ta@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "trajectory-user", "tu@example.com", "password", rbac.InvestorRoleCode)
	admin := loginHTTPTestUser(t, router, "trajectory-admin", "password")
	user := loginHTTPTestUser(t, router, "trajectory-user", "password")
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {user, 403}, {admin, 200}} {
		r := httptest.NewRequest("GET", "/api/v1/trajectory/devices", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if s.owner <= 0 {
		t.Fatal("owner missing")
	}
}
