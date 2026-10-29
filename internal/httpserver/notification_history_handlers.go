package httpserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/notification"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

// registerNotificationHistoryRoutes 注册通知发送历史查询接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供历史查询。
// 输出：无。
// 副作用：修改路由注册表。
func registerNotificationHistoryRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *notification.Service) {
	// 1. 依赖不完整时不注册，保持应用其他页面可用。
	if authService == nil || rbacService == nil || service == nil {
		return
	}
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/notifications/logs", rbac.PermissionFinanceNotifications, func(w http.ResponseWriter, request *http.Request) {
		// 2. 解析筛选条件并返回统一分页结构。
		filter, err := parseNotificationLogFilter(request)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		result, err := service.List(request.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "读取通知历史失败")
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// parseNotificationLogFilter 解析并校验通知历史查询参数。
// 输入：request 提供 q、status、category、page、page_size。
// 输出：返回规范化筛选条件；非法参数返回可读错误。
// 副作用：无。
func parseNotificationLogFilter(request *http.Request) (notification.LogFilter, error) {
	// 1. 设置默认分页并读取筛选值。
	query := request.URL.Query()
	filter := notification.LogFilter{
		Query:    strings.TrimSpace(query.Get("q")),
		Status:   strings.TrimSpace(query.Get("status")),
		Category: strings.TrimSpace(query.Get("category")),
		Page:     1,
		PageSize: 20,
	}
	if value := strings.TrimSpace(query.Get("page")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return filter, fmt.Errorf("页码必须是正整数")
		}
		filter.Page = parsed
	}
	if value := strings.TrimSpace(query.Get("page_size")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			return filter, fmt.Errorf("每页数量必须在 1-100 之间")
		}
		filter.PageSize = parsed
	}

	// 2. 限制搜索长度并校验固定枚举。
	if utf8.RuneCountInString(filter.Query) > 100 {
		return filter, fmt.Errorf("搜索词最多 100 个字符")
	}
	switch filter.Status {
	case "", "success", "failed", "unknown":
	default:
		return filter, fmt.Errorf("通知状态无效")
	}
	switch filter.Category {
	case "", notification.CategoryTask, notification.CategoryBackup, notification.CategoryMonitor,
		notification.CategorySubscription, notification.CategoryProject, notification.CategoryOther:
	default:
		return filter, fmt.Errorf("通知分类无效")
	}
	return filter, nil
}
