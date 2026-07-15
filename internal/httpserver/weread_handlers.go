package httpserver

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/weread"
)

// registerWeReadRoutes 注册微信读书看板、摘要、进度和热力图接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 聚合外部数据。
// 输出：无。
// 副作用：修改路由注册表。
func registerWeReadRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *weread.Service) {
	// 1. 给全部微信读书接口安装认证和页面权限中间件。
	router.Route("/api/v1/weread", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionWeread))
		routes.Get("/dashboard", wereadResponse(service.Dashboard))
		routes.Get("/summary", wereadResponse(service.Summary))
		routes.Get("/progress", wereadResponse(service.Progress))
		routes.Get("/heatmap", wereadResponse(service.Heatmap))
	})
}

// wereadResponse 把微信读书服务方法转换为 HTTP 处理器。
// 输入：load 是一个只读取外部数据的服务方法。
// 输出：返回统一处理成功和 502 错误的处理器。
// 副作用：处理请求时调用微信读书外部 HTTP API 并写入响应。
func wereadResponse(load func(context.Context) (map[string]any, error)) http.HandlerFunc {
	// 1. 返回复用的外部服务响应包装器。
	return func(w http.ResponseWriter, request *http.Request) {
		// 2. 调用服务并将外部错误转换为 Bad Gateway。
		response, err := load(request.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}
