package httpserver

import (
	"context"
	"encoding/json"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/receiptsplit"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type receiptStub struct{ err error }

func (s receiptStub) Statistics(_ context.Context, q receiptsplit.Query) (receiptsplit.Report, error) {
	return receiptsplit.Report{Version: 1, From: q.From, To: q.To, ExcludeOperators: q.ExcludeOperators}, s.err
}
func TestReceiptSplitAdminAndErrors(t *testing.T) {
	db := testdatabase.Open(t)
	rb := rbac.NewService(rbac.NewRepository(db))
	if err := rb.SyncDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("receipt-test-secret", time.Hour))
	router := NewRouter(Dependencies{Auth: a, RBAC: rb, ReceiptSplit: receiptStub{}})
	createHTTPTestUser(t, db, "receipt-admin", "ra@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "receipt-user", "ru@example.com", "password", rbac.InvestorRoleCode)
	admin := loginHTTPTestUser(t, router, "receipt-admin", "password")
	user := loginHTTPTestUser(t, router, "receipt-user", "password")
	if _, err := db.Exec(`INSERT INTO aowugong_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM aowugong_roles r,aowugong_permissions p WHERE r.code=? AND p.code=?`, rbac.InvestorRoleCode, rbac.PermissionReceiptSplit); err != nil {
		t.Fatal(err)
	}
	q := "?from=2026-10-21&to=2026-10-23"
	for _, tc := range []struct {
		token, q string
		status   int
	}{{"", q, 401}, {user, q, 403}, {admin, q, 200}, {admin, q + "&user=secret", 400}, {admin, q + "&exclude_operators=0", 400}, {admin, "?from=bad", 400}} {
		r := httptest.NewRequest("GET", "/api/v1/receipt-split/statistics"+tc.q, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%d != %d %s", w.Code, tc.status, w.Body.String())
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{receiptsplit.ErrNotConfigured, 503}, {receiptsplit.ErrUpstream, 502}} {
		router = NewRouter(Dependencies{Auth: a, RBAC: rb, ReceiptSplit: receiptStub{tc.err}})
		r := httptest.NewRequest("GET", "/api/v1/receipt-split/statistics"+q, nil)
		r.Header.Set("Authorization", "Bearer "+admin)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
	}
}

// 显式配置时才访问本机临时小票测试进程，不使用生产环境配置。
func TestReceiptSplitLiveContract(t *testing.T) {
	address := os.Getenv("RECEIPT_STATS_TEST_URL")
	if address == "" {
		t.Skip("temporary receipt server not supplied")
	}
	client, err := receiptsplit.NewClient(address, "receipt-stats-test-key-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	db := testdatabase.Open(t)
	rb := rbac.NewService(rbac.NewRepository(db))
	if err = rb.SyncDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("receipt-test-secret", time.Hour))
	router := NewRouter(Dependencies{Auth: a, RBAC: rb, ReceiptSplit: client})
	createHTTPTestUser(t, db, "contract-admin", "ca@example.com", "password", rbac.AdminRoleCode)
	token := loginHTTPTestUser(t, router, "contract-admin", "password")
	r := httptest.NewRequest("GET", "/api/v1/receipt-split/statistics?from=2026-10-21&to=2026-10-23&exclude_operators=true", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("contract %d %s", w.Code, w.Body.String())
	}
	var report receiptsplit.Report
	if err = json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.LifetimeLoginUsers != 1 || report.Totals.Tasks != 2 || report.Totals.AnalysisUsers != 1 || report.Totals.Bills != 1 || len(report.Days) != 2 {
		t.Fatalf("unexpected fixture %+v", report)
	}
}
