package httpserver

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

// notificationExternalConfig 描述外部服务调用通知 API 所需的固定配置。
type notificationExternalConfig struct {
	Endpoint           string `json:"endpoint"`
	Project            string `json:"project"`
	Configured         bool   `json:"configured"`
	Token              string `json:"token"`
	MaxMessageBytes    int    `json:"max_message_bytes"`
	MaxRequestBytes    int    `json:"max_request_bytes"`
	RateLimitPerMinute int    `json:"rate_limit_per_minute"`
}

// registerNotificationExternalRoutes 注册外部调用配置查询接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，tokens 是项目 Token。
// 输出：无。
// 副作用：修改路由注册表。
func registerNotificationExternalRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, tokens map[string]string) {
	// 1. 依赖不完整时不注册。
	if authService == nil || rbacService == nil {
		return
	}
	registerFinanceGET(router, authService, rbacService, "/api/v1/finance/notifications/external", rbac.PermissionFinanceNotifications, func(w http.ResponseWriter, _ *http.Request) {
		// 2. 只返回固定的 external 项目 Token，不暴露其他项目。
		writeJSON(w, http.StatusOK, buildNotificationExternalConfig(tokens))
	})
}

// buildNotificationExternalConfig 构造外部调用配置。
// 输入：tokens 是项目标识到 Token 的映射。
// 输出：返回固定 external 项目的端点和限制。
// 副作用：无。
func buildNotificationExternalConfig(tokens map[string]string) notificationExternalConfig {
	// 1. 只读取 external 项目，未配置时 token 为空。
	token := strings.TrimSpace(tokens["external"])
	return notificationExternalConfig{
		Endpoint:           "/api/v1/notifications/wechat",
		Project:            "external",
		Configured:         token != "",
		Token:              token,
		MaxMessageBytes:    2048,
		MaxRequestBytes:    16384,
		RateLimitPerMinute: 10,
	}
}
