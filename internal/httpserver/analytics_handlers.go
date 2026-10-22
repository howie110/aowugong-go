package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/analytics"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"golang.org/x/time/rate"
)

type analyticsService interface {
	Collect(context.Context, analytics.Payload, string, string, string) error
	Report(context.Context, analytics.Filter) (analytics.Report, error)
}

// registerAnalyticsRoutes 将公开采集与管理员读取分开，采集不读取账号和令牌。
func registerAnalyticsRoutes(router chi.Router, service analyticsService, authService *auth.Service, rbacService *rbac.Service, publicURL string, proxies []netip.Prefix) {
	if service == nil {
		return
	}
	limiter := rate.NewLimiter(30, 120)
	origin, _ := url.Parse(publicURL)
	router.Post("/api/v1/analytics/collect", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if origin == nil || origin.Host == "" || r.Header.Get("Origin") != origin.Scheme+"://"+origin.Host || !strings.EqualFold(r.Host, origin.Host) {
			writeError(w, 403, "forbidden", "采集来源无效")
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, 415, "unsupported_media_type", "需要 JSON")
			return
		}
		if !limiter.Allow() {
			writeError(w, 429, "rate_limited", "请求过于频繁")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var p analytics.Payload
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&p) != nil {
			writeError(w, 400, "invalid_request", "采集参数无效")
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, 400, "invalid_request", "采集参数无效")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		err := service.Collect(ctx, p, analyticsClientIP(r, proxies), r.UserAgent(), origin.Hostname())
		if errors.Is(err, analytics.ErrInvalid) {
			writeError(w, 400, "invalid_request", "采集参数无效")
			return
		}
		if err != nil {
			writeError(w, 503, "unavailable", "统计暂不可用")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if authService == nil || rbacService == nil {
		return
	}
	router.Route("/api/v1/analytics/report", func(routes chi.Router) {
		routes.Use(authenticate(authService), requireAdministrator(rbacService), requirePermission(rbacService, rbac.PermissionAnalytics))
		routes.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			f := analytics.Filter{Values: map[string]string{}}
			q := r.URL.Query()
			var err error
			f.From, err = time.Parse(time.RFC3339, q.Get("from"))
			if err == nil {
				f.To, err = time.Parse(time.RFC3339, q.Get("to"))
			}
			for k, v := range q {
				if k != "from" && k != "to" {
					if len(v) != 1 {
						err = analytics.ErrInvalid
					}
					f.Values[k] = v[0]
				}
			}
			if err != nil || f.Validate() != nil {
				writeError(w, 400, "invalid_request", "统计筛选无效，时间跨度最多 366 天")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			result, err := service.Report(ctx, f)
			if err != nil {
				writeError(w, 500, "internal_error", "读取访问统计失败")
				return
			}
			writeJSON(w, 200, result)
		})
	})
}

// analyticsClientIP 从右向左剥离可信代理，不能信任客户端伪造的最左 XFF。
func analyticsClientIP(r *http.Request, proxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	trusted := func(ip netip.Addr) bool {
		for _, p := range proxies {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer.String()
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		next, e := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if e != nil {
			return peer.String()
		}
		peer = next.Unmap()
		if !trusted(peer) {
			return peer.String()
		}
	}
	return peer.String()
}
