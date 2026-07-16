package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	financeservice "github.com/howiedata/aowugong-go/internal/finance/service"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

// registerFinanceRoutes 注册 finance 控制台和工具摘要接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供摘要业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerFinanceRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *financeservice.DashboardService) {
	// 1. 控制台兼容当前 React 使用的尾斜杠路径。
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/overview", rbac.PermissionFinanceOverview, func(w http.ResponseWriter, request *http.Request) {
		// 2. 查询控制台数据进度并转换服务错误。
		result, err := service.Overview(request.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "读取控制台摘要失败")
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/overview/", rbac.PermissionFinanceOverview, func(w http.ResponseWriter, request *http.Request) {
		// 3. 尾斜杠入口调用同一摘要服务，不复制业务规则。
		result, err := service.Overview(request.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "读取控制台摘要失败")
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// 4. 注册纯内存摘要接口。
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/backtest/summary", rbac.PermissionFinanceBacktest, func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, http.StatusOK, service.BacktestSummary())
	})
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/jobs/summary", rbac.PermissionFinanceJobs, func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, http.StatusOK, service.JobsSummary())
	})
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/trading/summary", rbac.PermissionFinanceTrading, func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, http.StatusOK, service.TradingSummary())
	})
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/notifications/summary", rbac.PermissionFinanceNotifications, func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, http.StatusOK, service.NotificationsSummary())
	})

	// 5. 数据摘要查询 MySQL 并统一处理失败响应。
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/data/summary", rbac.PermissionFinanceData, func(w http.ResponseWriter, request *http.Request) {
		result, err := service.DataSummary(request.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "读取数据摘要失败")
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// registerFinanceGET 给单个 finance GET 接口安装认证和页面权限中间件。
// 输入：router 是路由器，服务提供访问控制，path 和 permission 描述接口，handler 处理成功请求。
// 输出：无。
// 副作用：修改路由注册表。
func registerFinanceGET(router chi.Router, authService *auth.Service, rbacService *rbac.Service, path, permission string, handler http.HandlerFunc) {
	// 1. 用独立路由组确保每个页面权限明确可审计。
	router.With(authenticate(authService), requirePermission(rbacService, permission)).Get(path, handler)
}
