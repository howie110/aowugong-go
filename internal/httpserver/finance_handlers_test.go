package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	"github.com/howiedata/aowugong-go/internal/finance/position"
	financeservice "github.com/howiedata/aowugong-go/internal/finance/service"
	"github.com/howiedata/aowugong-go/internal/finance/stockanalysis"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestFinanceSummaryRoutesRequirePermissionAndReturnCurrentData 验证 finance 摘要路由受权限保护并读取 SQLite。
// 输入：管理员和投资者测试用户、隔离 SQLite。
// 输出：管理员获得 200，缺少控制台权限的投资者获得 403。
// 副作用：创建隔离 SQLite 并写入测试用户。
func TestFinanceSummaryRoutesRequirePermissionAndReturnCurrentData(t *testing.T) {
	// 1. 创建完整迁移数据库并组装 finance 路由依赖。
	ctx := context.Background()
	db := testdatabase.Open(t)
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("finance-http-secret", 72*time.Hour))
	financeService := financeservice.NewDashboardService(db, financeservice.DashboardOptions{HTTPAddress: "0.0.0.0:2346"})
	positionRepository := position.NewRepository(db)
	positionService := position.NewService(positionRepository, client.NewAliyunOCRClient(config.PositionOCR{}), position.UploadOptions{})
	stockService := stockanalysis.NewService(stockanalysis.NewRepository(db))
	articleRepository := articleanalysis.NewRepository(db)
	if err := articleRepository.SyncDefaultSource(ctx, ""); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	articleService := articleanalysis.NewService(articleRepository, articleanalysis.ServiceOptions{Model: "test-model"})
	jobRegistry := scheduler.NewRegistry(db, nil, nil)
	if err := jobRegistry.Register(scheduler.Definition{
		Name: "test_job", Schedule: "0 9 * * *", Timeout: time.Minute,
		Run: func(context.Context) (string, error) { return "manual-ok", nil },
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	handler := NewRouter(Dependencies{
		Auth: authService, RBAC: rbacService, Finance: financeService,
		Position: positionService, StockAnalysis: stockService, ArticleAnalysis: articleService,
		Jobs: jobRegistry,
	})

	// 2. 创建两个角色用户并登录。
	createHTTPTestUser(t, db, "admin", "admin-finance@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "investor", "investor-finance@example.com", "password", rbac.InvestorRoleCode)
	adminToken := loginHTTPTestUser(t, handler, "admin", "password")
	investorToken := loginHTTPTestUser(t, handler, "investor", "password")

	// 3. 管理员读取数据摘要必须成功且响应包含 SQLite 数据源。
	adminRequest := httptest.NewRequest(http.MethodGet, "/api/v1/finance/data/summary", nil)
	adminRequest.Header.Set("Authorization", "Bearer "+adminToken)
	adminRecorder := httptest.NewRecorder()
	handler.ServeHTTP(adminRecorder, adminRequest)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body = %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	for _, path := range []string{
		"/api/v1/finance/positions/summary",
		"/api/v1/finance/positions/snapshots/recent?limit=20",
		"/api/v1/finance/stock-analysis/summary",
		"/api/v1/finance/stock-analysis/report?limit=500",
		"/api/v1/finance/article-analysis/summary",
		"/api/v1/finance/article-analysis/fetch-summary",
		"/api/v1/finance/article-analysis/sources",
		"/api/v1/finance/article-analysis/articles?days=60&limit=50",
		"/api/v1/finance/article-analysis/report?target_days=60&market_days=3",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}
	if body := adminRecorder.Body.String(); !containsAll(body, `"name":"PostgreSQL"`, `"title":"数据"`) {
		t.Errorf("admin body = %s, want PostgreSQL data summary", body)
	}
	manualRequest := httptest.NewRequest(http.MethodPost, "/api/v1/finance/jobs/test_job/run", nil)
	manualRequest.Header.Set("Authorization", "Bearer "+adminToken)
	manualRecorder := httptest.NewRecorder()
	handler.ServeHTTP(manualRecorder, manualRequest)
	if manualRecorder.Code != http.StatusOK || !strings.Contains(manualRecorder.Body.String(), `"message":"manual-ok"`) {
		t.Errorf("manual job status = %d, body = %s", manualRecorder.Code, manualRecorder.Body.String())
	}

	// 4. 投资者缺少数据页权限时必须得到 403。
	investorRequest := httptest.NewRequest(http.MethodGet, "/api/v1/finance/data/summary", nil)
	investorRequest.Header.Set("Authorization", "Bearer "+investorToken)
	investorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(investorRecorder, investorRequest)
	if investorRecorder.Code != http.StatusForbidden {
		t.Errorf("investor status = %d, want %d", investorRecorder.Code, http.StatusForbidden)
	}
}

// containsAll 检查文本是否包含全部测试片段。
// 输入：value 是响应正文，parts 是必须出现的片段。
// 输出：全部出现时返回 true。
// 副作用：无。
func containsAll(value string, parts ...string) bool {
	// 1. 逐项检查，发现缺失立即返回 false。
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
