package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type permissionHandlers struct {
	service *rbac.Service
}

// registerPermissionRoutes 注册受页面权限保护的 RBAC 管理接口。
// 输入：router 是 API 路由器，authService 用于认证，service 用于权限业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerPermissionRoutes(router chi.Router, authService *auth.Service, service *rbac.Service) {
	// 1. 为权限接口统一安装认证和页面权限中间件。
	handlers := permissionHandlers{service: service}
	router.Route("/api/v1/permissions", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(service, rbac.PermissionPermissions))
		routes.Get("/summary", handlers.summary)
		routes.Get("/users", handlers.users)
		routes.Get("/roles", handlers.roles)
		routes.Post("/users/{userID}/roles", handlers.assignRole)
	})
}

// summary 返回权限管理页面只读摘要。
// 输入：request 已通过认证和权限校验。
// 输出：写入页面摘要 JSON。
// 副作用：写入 HTTP 响应。
func (h permissionHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 返回与旧页面契约一致的摘要结构。
	writeJSON(w, http.StatusOK, map[string]any{
		"title":       "权限管理",
		"description": "把用户加入角色，角色权限由系统预设维护。",
		"metrics": []map[string]string{
			{"label": "角色模型", "value": "RBAC", "detail": "用户 -> 角色 -> 权限", "status": "normal"},
			{"label": "管理员", "value": "全权限", "detail": "管理员天然拥有所有权限", "status": "normal"},
		},
	})
}

// users 返回用户及其角色列表。
// 输入：request 已通过认证和权限校验。
// 输出：写入 UserRoles 数组。
// 副作用：读取 SQLite 并写入 HTTP 响应。
func (h permissionHandlers) users(w http.ResponseWriter, request *http.Request) {
	// 1. 调用服务读取权限管理用户列表。
	users, err := h.service.ListUsers(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取用户列表失败")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// roles 返回可分配的启用角色。
// 输入：request 已通过认证和权限校验。
// 输出：写入 Role 数组。
// 副作用：同步并读取 SQLite，写入 HTTP 响应。
func (h permissionHandlers) roles(w http.ResponseWriter, request *http.Request) {
	// 1. 同步代码基线后读取可分配角色。
	roles, err := h.service.ListRoles(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取角色列表失败")
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

// assignRole 幂等地给目标用户添加角色。
// 输入：路径包含 userID，请求体包含 role_code。
// 输出：成功写入更新后的 UserRoles。
// 副作用：写入并读取 SQLite，写入 HTTP 响应。
func (h permissionHandlers) assignRole(w http.ResponseWriter, request *http.Request) {
	// 1. 校验路径主键和 JSON 请求体。
	userID, err := strconv.ParseInt(chi.URLParam(request, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户编号无效")
		return
	}
	var payload struct {
		RoleCode string `json:"role_code"`
	}
	if err := decodeJSON(w, request, &payload); err != nil || payload.RoleCode == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "角色请求无效")
		return
	}

	// 2. 分配角色并转换用户或角色不存在错误。
	if err := h.service.AssignRole(request.Context(), userID, payload.RoleCode); err != nil {
		if errors.Is(err, rbac.ErrUserNotFound) || errors.Is(err, rbac.ErrRoleNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "分配角色失败")
		return
	}

	// 3. 从统一用户列表中返回目标用户的最新角色结构。
	users, err := h.service.ListUsers(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取用户角色失败")
		return
	}
	for _, user := range users {
		if user.ID == userID {
			writeJSON(w, http.StatusOK, user)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "用户不存在")
}
