package httpserver

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/scheduler"
)

// registerJobRoutes 注册任务定义查询和手动执行接口。
// 输入：router 是 API 路由，authService、rbacService 控制访问，registry 是统一任务入口。
// 输出：无。
// 副作用：修改路由注册表；请求执行时可能运行任务、写库和发送通知。
func registerJobRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, registry *scheduler.Registry) {
	// 1. 查询接口只暴露任务名称、说明、频率和超时，不暴露函数或密钥。
	router.With(authenticate(authService), requirePermission(rbacService, rbac.PermissionFinanceJobs)).Get(
		"/api/v1/finance/jobs/definitions", func(w http.ResponseWriter, request *http.Request) {
			definitions := registry.Definitions()
			items := make([]map[string]any, 0, len(definitions))
			for _, definition := range definitions {
				items = append(items, map[string]any{
					"name": definition.Name, "description": definition.Description,
					"schedule": definition.Schedule, "timeout_seconds": int(definition.Timeout.Seconds()),
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": items})
		},
	)

	// 2. 手动接口从路径读取已注册名称并调用同一个包装器。
	router.With(authenticate(authService), requirePermission(rbacService, rbac.PermissionFinanceJobs)).Post(
		"/api/v1/finance/jobs/{name}/run", func(w http.ResponseWriter, request *http.Request) {
			result, err := registry.Run(request.Context(), chi.URLParam(request, "name"), scheduler.SourceManual)
			if err != nil {
				if errors.Is(err, scheduler.ErrAlreadyRunning) {
					writeError(w, http.StatusConflict, "job_running", err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "job_failed", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, result)
		},
	)
}
