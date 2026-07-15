// Package httpserver 提供 HTTP 路由与 SPA 静态文件服务。
package httpserver

import (
	"net/http"
	"strings"
)

// Dependencies 描述路由器启动所需的依赖。
type Dependencies struct {
	StaticDir string
}

type router struct {
	spa spaHandler
}

// NewRouter 创建应用的 HTTP 路由器。
func NewRouter(deps Dependencies) http.Handler {
	// 1. 组装 API 路由与 SPA 静态文件处理器。
	return router{spa: newSPAHandler(deps.StaticDir)}
}

// ServeHTTP 按请求路径分发 API 和 SPA 请求。
func (r router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	// 1. 处理已注册的健康检查端点。
	if request.Method == http.MethodGet && request.URL.Path == "/api/v1/health" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// 2. 阻止未知 API 请求进入 SPA 回退。
	if strings.HasPrefix(request.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "API route not found")
		return
	}

	// 3. 将其他请求交给静态文件与 SPA 处理器。
	r.spa.ServeHTTP(w, request)
}
