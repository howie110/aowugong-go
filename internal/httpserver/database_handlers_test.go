package httpserver

import (
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/databaseview"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type databaseRouteStub struct{}

// Summary 返回数据库路由测试使用的固定 PostgreSQL 概况。
// 输入：ctx 是请求上下文。
// 输出：返回一张用户表的概况。
// 副作用：无。
func (databaseRouteStub) Summary(ctx context.Context) (databaseview.Summary, error) {
	return databaseview.Summary{
		Engine: "PostgreSQL", JournalMode: "WAL", TableCount: 1,
		Tables: []databaseview.TableSummary{{Name: "aowugong_fastapi_users", RowCount: 1}},
	}, nil
}

// Rows 返回数据库路由测试使用的脱敏用户数据。
// 输入：ctx 是请求上下文，其余参数描述表、搜索和分页。
// 输出：返回一条密码已隐藏的用户记录。
// 副作用：无。
func (databaseRouteStub) Rows(
	ctx context.Context, table, search string, page, pageSize int,
) (databaseview.RowsPage, error) {
	return databaseview.RowsPage{
		Table: table, Page: page, PageSize: pageSize, Total: 1,
		Rows: []map[string]any{{"username": "db-admin", "password": "已隐藏"}},
	}, nil
}

// ExportCSV 写入数据库路由测试使用的脱敏用户 CSV。
// 输入：ctx 是请求上下文，table 和 search 描述导出范围，output 接收 CSV。
// 输出：成功返回 nil；写入失败时返回错误。
// 副作用：向 output 写入 CSV 内容。
func (databaseRouteStub) ExportCSV(
	ctx context.Context, table, search string, output io.Writer,
) error {
	writer := csv.NewWriter(output)
	if err := writer.Write([]string{"username", "password"}); err != nil {
		return err
	}
	if err := writer.Write([]string{"db-admin", "已隐藏"}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

// TestDatabaseRoutesRequireAdminPermissionAndReturnRedactedRows 验证数据库页面仅管理员可读。
// 输入：管理员和投资者、隔离 SQLite 用户表。
// 输出：管理员获得概况和脱敏行，投资者获得 403。
// 副作用：创建并写入隔离 SQLite 测试库，执行 HTTP 请求。
func TestDatabaseRoutesRequireAdminPermissionAndReturnRedactedRows(t *testing.T) {
	// 1. 创建完整 SQLite、权限服务和只读数据库路由。
	ctx := context.Background()
	db := testdatabase.Open(t)
	rbacService := rbac.NewService(rbac.NewRepository(db))
	if err := rbacService.SyncDefaults(ctx); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.NewTokenManager("database-http-secret", 72*time.Hour))
	handler := NewRouter(Dependencies{
		Auth: authService, RBAC: rbacService, Database: databaseRouteStub{},
	})

	// 2. 创建管理员和投资者并取得令牌。
	createHTTPTestUser(t, db, "db-admin", "db-admin@example.com", "password", rbac.AdminRoleCode)
	createHTTPTestUser(t, db, "db-investor", "db-investor@example.com", "password", rbac.InvestorRoleCode)
	adminToken := loginHTTPTestUser(t, handler, "db-admin", "password")
	investorToken := loginHTTPTestUser(t, handler, "db-investor", "password")

	// 3. 管理员可读取概况和已隐藏密码的数据行。
	for _, path := range []string{
		"/api/v1/database/summary",
		"/api/v1/database/tables/aowugong_fastapi_users?page=1&page_size=20&search=db-admin",
		"/api/v1/database/tables/aowugong_fastapi_users/export?search=db-admin",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(path, "/export") {
			if !containsAll(recorder.Body.String(), "db-admin", "已隐藏") ||
				strings.Contains(recorder.Body.String(), "$2") {
				t.Errorf("export body = %s", recorder.Body.String())
			}
		} else if path != "/api/v1/database/summary" &&
			(!containsAll(recorder.Body.String(), `"username":"db-admin"`, `"password":"已隐藏"`)) {
			t.Errorf("rows body = %s", recorder.Body.String())
		}
	}

	// 4. 投资者没有数据库权限。
	request := httptest.NewRequest(http.MethodGet, "/api/v1/database/summary", nil)
	request.Header.Set("Authorization", "Bearer "+investorToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("investor status = %d, want 403", recorder.Code)
	}

	// 5. 即使管理员也没有数据库写入路由。
	writeRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/database/tables/aowugong_fastapi_users", nil,
	)
	writeRequest.Header.Set("Authorization", "Bearer "+adminToken)
	writeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(writeRecorder, writeRequest)
	if writeRecorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("database write status = %d, want 405", writeRecorder.Code)
	}
}
