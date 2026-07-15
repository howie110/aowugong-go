package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

// registerMonitoringRoutes 注册监控摘要和手动检查接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供监控业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerMonitoringRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *monitoring.Service) {
	// 1. 给全部监控接口安装认证和页面权限中间件。
	router.Route("/api/v1/monitoring", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionMonitoring))
		routes.Get("/summary", func(w http.ResponseWriter, request *http.Request) {
			// 2. 读取最近一次结果并构建页面摘要。
			summary, err := service.Summary(request.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "读取监控摘要失败")
				return
			}
			writeJSON(w, http.StatusOK, summary)
		})
		routes.Post("/check", func(w http.ResponseWriter, request *http.Request) {
			// 3. 手动执行与定时任务相同的全目标检查服务。
			result, err := service.CheckAll(request.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "执行服务监控失败")
				return
			}
			writeJSON(w, http.StatusOK, result)
		})
	})
}
