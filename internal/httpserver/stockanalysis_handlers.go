package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/finance/stockanalysis"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type stockAnalysisHandlers struct {
	service *stockanalysis.Service
}

// registerStockAnalysisRoutes 注册股票仓位分析摘要和完整报告接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供分析业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerStockAnalysisRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *stockanalysis.Service) {
	// 1. 给全部分析接口安装认证和股票分析页面权限。
	handlers := stockAnalysisHandlers{service: service}
	router.Route("/api/v1/finance/stock-analysis", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionFinanceStockAnalysis))
		routes.Get("/summary", handlers.summary)
		routes.Get("/report", handlers.report)
	})
}

// summary 返回股票仓位分析页顶部摘要。
// 输入：request 已通过认证和股票分析权限校验。
// 输出：写入 Summary JSON。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h stockAnalysisHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 生成摘要并统一转换服务错误。
	result, err := h.service.Summary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取股票仓位摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// report 返回股票仓位完整分析报告。
// 输入：查询参数 limit 可选，范围 1 到 2000。
// 输出：写入 Report JSON。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h stockAnalysisHandlers) report(w http.ResponseWriter, request *http.Request) {
	// 1. 解析并限制快照查询数量。
	limit := 500
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 2000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit 必须在 1 到 2000 之间")
			return
		}
		limit = parsed
	}

	// 2. 生成报告并统一转换服务错误。
	result, err := h.service.Report(request.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取股票仓位报告失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
