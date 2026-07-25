package httpserver

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// NewDevelopmentRouter 创建本地静态资源和线上 API 组合路由。
// 输入：staticDir 是本地前端产物，upstreamURL 是线上 Go 服务根地址。
// 输出：返回本地页面使用的处理器；上游地址不安全或无效时返回错误。
// 副作用：处理请求时把全部 /api 流量转发到远端服务。
func NewDevelopmentRouter(staticDir, upstreamURL string) (http.Handler, error) {
	// 1. 只接受不含身份、查询和业务路径的 HTTP(S) 根地址。
	target, err := url.Parse(strings.TrimSpace(upstreamURL))
	if err != nil {
		return nil, fmt.Errorf("解析开发 API 上游: %w", err)
	}
	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" ||
		target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
		(target.Path != "" && target.Path != "/") {
		return nil, fmt.Errorf("开发 API 上游必须是 HTTP(S) 根地址")
	}
	target.Path = ""

	// 2. 使用标准反向代理保留请求路径、查询、令牌和响应状态。
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, request *http.Request, proxyErr error) {
		// 3. 上游不可达时返回与应用一致的 JSON 错误。
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "线上 API 暂时不可用")
	}
	return router{api: proxy, spa: newSPAHandler(staticDir)}, nil
}
