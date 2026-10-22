package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/receiptsplit"
)

type receiptStatisticsService interface {
	Statistics(context.Context, receiptsplit.Query) (receiptsplit.Report, error)
}

// registerReceiptSplitRoutes 双重检查管理员和页面权限，浏览器只接触汇总结果。
func registerReceiptSplitRoutes(router chi.Router, service receiptStatisticsService, authService *auth.Service, rbacService *rbac.Service) {
	if service == nil || authService == nil || rbacService == nil {
		return
	}
	router.Route("/api/v1/receipt-split", func(routes chi.Router) {
		routes.Use(authenticate(authService), requireAdministrator(rbacService), requirePermission(rbacService, rbac.PermissionReceiptSplit))
		routes.Get("/statistics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			values := r.URL.Query()
			q := receiptsplit.Query{From: values.Get("from"), To: values.Get("to"), ExcludeOperators: true}
			for k, v := range values {
				if (k != "from" && k != "to" && k != "exclude_operators") || len(v) != 1 {
					writeError(w, 400, "invalid_request", "统计参数无效")
					return
				}
			}
			if v, ok := values["exclude_operators"]; ok {
				if v[0] != "true" && v[0] != "false" {
					writeError(w, 400, "invalid_request", "统计排除参数无效")
					return
				}
				q.ExcludeOperators = v[0] == "true"
			}
			if q.Validate() != nil {
				writeError(w, 400, "invalid_request", "日期范围无效，最多 366 天")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
			defer cancel()
			result, err := service.Statistics(ctx, q)
			if errors.Is(err, receiptsplit.ErrNotConfigured) {
				writeError(w, 503, "not_configured", "尚未配置小票分账统计连接")
				return
			}
			if err != nil {
				writeError(w, 502, "upstream_unavailable", "小票分账统计暂不可用，请检查服务和统计连接配置")
				return
			}
			writeJSON(w, 200, result)
		})
	})
}
