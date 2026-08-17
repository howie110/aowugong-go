// Package httpserver 提供 HTTP 路由与 SPA 静态文件服务。
package httpserver

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	"github.com/howiedata/aowugong-go/internal/finance/position"
	financeservice "github.com/howiedata/aowugong-go/internal/finance/service"
	"github.com/howiedata/aowugong-go/internal/finance/stockanalysis"
	"github.com/howiedata/aowugong-go/internal/mahjong"
	"github.com/howiedata/aowugong-go/internal/monitoring"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/scheduler"
	"github.com/howiedata/aowugong-go/internal/subscription"
	"github.com/howiedata/aowugong-go/internal/vpn"
	"github.com/howiedata/aowugong-go/internal/weread"
	"github.com/howiedata/aowugong-go/internal/work"
)

// Dependencies 描述路由器启动所需的依赖。
type Dependencies struct {
	StaticDir       string
	Auth            *auth.Service
	RBAC            *rbac.Service
	Subscription    *subscription.Service
	Mahjong         *mahjong.Service
	Work            *work.Service
	WeRead          *weread.Service
	Monitoring      *monitoring.Service
	Finance         *financeservice.DashboardService
	Position        *position.Service
	StockAnalysis   *stockanalysis.Service
	ArticleAnalysis *articleanalysis.Service
	Jobs            *scheduler.Registry
	Database        databaseReadService
	VPN             *vpn.Service
}

type router struct {
	api http.Handler
	spa spaHandler
}

// NewRouter 创建应用的 HTTP 路由器。
// 输入：deps 包含静态目录和可选业务服务。
// 输出：返回组合 API 与 SPA 的 HTTP 处理器。
// 副作用：无，只构造路由注册表。
func NewRouter(deps Dependencies) http.Handler {
	// 1. 创建 API 路由并统一注册 JSON 404 和方法错误。
	api := chi.NewRouter()
	api.NotFound(func(w http.ResponseWriter, request *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "API route not found")
	})
	api.MethodNotAllowed(func(w http.ResponseWriter, request *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	})
	api.Get("/api/v1/health", healthHandler)
	if deps.ArticleAnalysis != nil {
		registerArticleFeedRoutes(api, deps.ArticleAnalysis)
	}

	// 2. 依赖可用时注册认证和权限接口，便于最小健康检查测试独立运行。
	if deps.Auth != nil {
		registerAuthRoutes(api, deps.Auth)
	}
	if deps.Auth != nil && deps.RBAC != nil {
		registerPermissionRoutes(api, deps.Auth, deps.RBAC)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Subscription != nil {
		registerSubscriptionRoutes(api, deps.Auth, deps.RBAC, deps.Subscription)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Mahjong != nil {
		registerMahjongRoutes(api, deps.Auth, deps.RBAC, deps.Mahjong)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Work != nil {
		registerWorkRoutes(api, deps.Auth, deps.RBAC, deps.Work)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.WeRead != nil {
		registerWeReadRoutes(api, deps.Auth, deps.RBAC, deps.WeRead)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Monitoring != nil {
		registerMonitoringRoutes(api, deps.Auth, deps.RBAC, deps.Monitoring)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Finance != nil {
		registerFinanceRoutes(api, deps.Auth, deps.RBAC, deps.Finance)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Position != nil {
		registerPositionRoutes(api, deps.Auth, deps.RBAC, deps.Position)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.StockAnalysis != nil {
		registerStockAnalysisRoutes(api, deps.Auth, deps.RBAC, deps.StockAnalysis)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.ArticleAnalysis != nil {
		registerArticleAnalysisRoutes(api, deps.Auth, deps.RBAC, deps.ArticleAnalysis)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Jobs != nil {
		registerJobRoutes(api, deps.Auth, deps.RBAC, deps.Jobs)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.Database != nil {
		registerDatabaseRoutes(api, deps.Auth, deps.RBAC, deps.Database)
	}
	if deps.Auth != nil && deps.RBAC != nil && deps.VPN != nil {
		registerVPNRoutes(api, deps.Auth, deps.RBAC, deps.VPN)
	}

	// 3. 组装 API 与 SPA 静态文件处理器。
	return router{api: api, spa: newSPAHandler(deps.StaticDir)}
}

// ServeHTTP 按请求路径分发 API 和 SPA 请求。
// 输入：w 接收响应，request 是当前 HTTP 请求。
// 输出：无。
// 副作用：调用 API 或静态资源处理器并写响应。
func (r router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	// 1. 将全部 API 请求交给 chi 路由，阻止进入 SPA 回退。
	if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") || strings.HasPrefix(request.URL.Path, "/feeds/") {
		r.api.ServeHTTP(w, request)
		return
	}

	// 2. 将其他请求交给静态文件与 SPA 处理器。
	r.spa.ServeHTTP(w, request)
}

// healthHandler 返回进程级健康状态。
// 输入：request 是健康检查请求。
// 输出：写入 status=ok 的 JSON。
// 副作用：写入 HTTP 响应。
func healthHandler(w http.ResponseWriter, request *http.Request) {
	// 1. 返回不依赖外部服务的存活状态。
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
