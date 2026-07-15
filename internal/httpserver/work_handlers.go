package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/work"
)

// registerWorkRoutes 注册私有工作导航接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 读取私有文件。
// 输出：无。
// 副作用：修改路由注册表。
func registerWorkRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *work.Service) {
	// 1. 工作导航包含私有链接，统一要求认证和页面权限。
	router.With(
		authenticate(authService),
		requirePermission(rbacService, rbac.PermissionWork),
	).Get("/api/v1/work/navigation", func(w http.ResponseWriter, request *http.Request) {
		// 2. 读取私有 JSON 并返回清洗后的导航。
		navigation, err := service.Navigation()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "读取工作导航失败")
			return
		}
		writeJSON(w, http.StatusOK, navigation)
	})
}
