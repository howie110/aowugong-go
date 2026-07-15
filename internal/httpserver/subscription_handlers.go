package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/subscription"
)

type subscriptionHandlers struct {
	service *subscription.Service
}

// registerSubscriptionRoutes 注册订阅摘要和记录 CRUD 接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供业务能力。
// 输出：无。
// 副作用：修改路由注册表。
func registerSubscriptionRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *subscription.Service) {
	// 1. 给全部订阅接口安装认证和页面权限中间件。
	handlers := subscriptionHandlers{service: service}
	router.Route("/api/v1/subscriptions", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionSubscriptions))
		routes.Get("/summary", handlers.summary)
		routes.Get("/records", handlers.list)
		routes.Post("/records", handlers.create)
		routes.Put("/records/{recordID}", handlers.update)
		routes.Delete("/records/{recordID}", handlers.delete)
	})
}

// summary 返回订阅页面摘要。
// 输入：request 已通过认证和订阅权限校验。
// 输出：写入 Summary。
// 副作用：读取 SQLite，空表时写入默认记录，并写入 HTTP 响应。
func (h subscriptionHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 调用统一服务生成摘要。
	summary, err := h.service.Summary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取订阅摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// list 返回全部订阅记录。
// 输入：request 已通过认证和订阅权限校验。
// 输出：写入 Record 数组。
// 副作用：读取 SQLite，空表时写入默认记录，并写入 HTTP 响应。
func (h subscriptionHandlers) list(w http.ResponseWriter, request *http.Request) {
	// 1. 从服务读取带实时状态的列表。
	records, err := h.service.List(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取订阅记录失败")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// create 新增订阅记录。
// 输入：request 包含 JSON 可编辑字段和认证用户。
// 输出：写入新建 Record。
// 副作用：写入 SQLite 和 HTTP 响应。
func (h subscriptionHandlers) create(w http.ResponseWriter, request *http.Request) {
	// 1. 解码页面提交字段并读取创建用户名。
	var payload subscription.WriteRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅请求无效")
		return
	}
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}

	// 2. 调用服务并转换业务错误。
	record, err := h.service.Create(request.Context(), payload, user.Username)
	if err != nil {
		writeSubscriptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// update 全量更新订阅记录。
// 输入：路径包含 recordID，请求体包含全部可编辑字段。
// 输出：写入更新后 Record。
// 副作用：写入 SQLite 和 HTTP 响应。
func (h subscriptionHandlers) update(w http.ResponseWriter, request *http.Request) {
	// 1. 校验路径主键并解码 JSON 请求。
	recordID, err := strconv.ParseInt(chi.URLParam(request, "recordID"), 10, 64)
	if err != nil || recordID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅编号无效")
		return
	}
	var payload subscription.WriteRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅请求无效")
		return
	}

	// 2. 调用统一更新服务并返回记录。
	record, err := h.service.Update(request.Context(), recordID, payload)
	if err != nil {
		writeSubscriptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// delete 删除订阅记录。
// 输入：路径包含 recordID。
// 输出：写入 deleted=true。
// 副作用：删除 SQLite 记录并写入 HTTP 响应。
func (h subscriptionHandlers) delete(w http.ResponseWriter, request *http.Request) {
	// 1. 校验路径主键并调用删除服务。
	recordID, err := strconv.ParseInt(chi.URLParam(request, "recordID"), 10, 64)
	if err != nil || recordID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅编号无效")
		return
	}
	deleted, err := h.service.Delete(request.Context(), recordID)
	if err != nil {
		writeSubscriptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": deleted})
}

// writeSubscriptionError 把订阅业务错误转换为 HTTP 响应。
// 输入：err 是订阅 service 返回的带上下文错误。
// 输出：无。
// 副作用：写入 HTTP 错误响应。
func writeSubscriptionError(w http.ResponseWriter, err error) {
	// 1. 区分参数、冲突、不存在和内部错误。
	switch {
	case errors.Is(err, subscription.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅参数无效")
	case errors.Is(err, subscription.ErrConflict):
		writeError(w, http.StatusBadRequest, "conflict", "订阅服务名已存在")
	case errors.Is(err, subscription.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "订阅记录不存在")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "订阅服务暂时不可用")
	}
}
