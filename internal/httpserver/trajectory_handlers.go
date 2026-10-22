package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/trajectory"
	"golang.org/x/time/rate"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type trajectoryService interface {
	Collect(context.Context, string, trajectory.Point) (bool, error)
	Devices(context.Context, int64) ([]trajectory.Device, error)
	Create(context.Context, int64, string) (trajectory.Credential, error)
	Rotate(context.Context, int64, string) (trajectory.Credential, error)
	Revoke(context.Context, int64, string) error
	Report(context.Context, int64, trajectory.Query) (trajectory.Report, error)
}

func trajectoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trajectory.ErrUnauthorized):
		writeError(w, 401, "unauthorized", "设备密钥无效或已撤销")
	case errors.Is(err, trajectory.ErrNotFound):
		writeError(w, 404, "not_found", "设备不存在或不可操作")
	case errors.Is(err, trajectory.ErrInvalid):
		writeError(w, 400, "invalid_request", "轨迹参数无效，请检查坐标、时间和筛选条件")
	case errors.Is(err, trajectory.ErrLimit):
		writeError(w, 409, "device_limit", "每个账号最多创建 20 台设备")
	default:
		writeError(w, 503, "unavailable", "轨迹服务暂不可用，请稍后重试")
	}
}
func registerTrajectoryRoutes(router chi.Router, s trajectoryService, a *auth.Service, rb *rbac.Service) {
	if s == nil {
		return
	}
	limiter := rate.NewLimiter(20, 100)
	slots := make(chan struct{}, 8)
	router.Route("/api/v1/trajectory", func(routes chi.Router) {
		routes.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Referrer-Policy", "no-referrer")
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
		routes.Post("/collect", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				trajectoryError(w, trajectory.ErrInvalid)
				return
			}
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != 64 {
				trajectoryError(w, trajectory.ErrUnauthorized)
				return
			}
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "5")
				writeError(w, 429, "rate_limited", "请降低上传频率后重试")
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				w.Header().Set("Retry-After", "5")
				writeError(w, 503, "busy", "请稍后重试")
				return
			}
			content, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || content != "application/x-www-form-urlencoded" {
				writeError(w, 415, "unsupported_media_type", "需要 form 请求正文")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
			if e = r.ParseForm(); e != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(e, &tooLarge) {
					writeError(w, 413, "too_large", "轨迹请求过大")
				} else {
					trajectoryError(w, trajectory.ErrInvalid)
				}
				return
			}
			p, e := trajectory.Parse(r.PostForm, time.Now().UTC())
			if e != nil {
				trajectoryError(w, e)
				return
			}
			inserted, e := s.Collect(r.Context(), parts[1], p)
			if e != nil {
				trajectoryError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "inserted": inserted})
		})
		if a == nil || rb == nil {
			return
		}
		routes.Group(func(admin chi.Router) {
			admin.Use(authenticate(a), requireAdministrator(rb), requirePermission(rb, rbac.PermissionTrajectory))
			admin.Get("/devices", func(w http.ResponseWriter, r *http.Request) {
				u, _ := currentUser(r)
				result, e := s.Devices(r.Context(), u.ID)
				if e != nil {
					trajectoryError(w, e)
					return
				}
				writeJSON(w, 200, result)
			})
			admin.Post("/devices", func(w http.ResponseWriter, r *http.Request) {
				r.Body = http.MaxBytesReader(w, r.Body, 1024)
				var body struct {
					Name string `json:"name"`
				}
				dec := json.NewDecoder(r.Body)
				dec.DisallowUnknownFields()
				if dec.Decode(&body) != nil || dec.Decode(&struct{}{}) != io.EOF {
					trajectoryError(w, trajectory.ErrInvalid)
					return
				}
				u, _ := currentUser(r)
				result, e := s.Create(r.Context(), u.ID, body.Name)
				if e != nil {
					trajectoryError(w, e)
					return
				}
				writeJSON(w, 201, result)
			})
			admin.Post("/devices/{id}/rotate", func(w http.ResponseWriter, r *http.Request) {
				u, _ := currentUser(r)
				result, e := s.Rotate(r.Context(), u.ID, chi.URLParam(r, "id"))
				if e != nil {
					trajectoryError(w, e)
					return
				}
				writeJSON(w, 200, result)
			})
			admin.Post("/devices/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
				u, _ := currentUser(r)
				if e := s.Revoke(r.Context(), u.ID, chi.URLParam(r, "id")); e != nil {
					trajectoryError(w, e)
					return
				}
				writeJSON(w, 200, map[string]bool{"ok": true})
			})
			admin.Get("/points", func(w http.ResponseWriter, r *http.Request) {
				v := r.URL.Query()
				for k, x := range v {
					if len(x) != 1 || (k != "from" && k != "to" && k != "device_id" && k != "page" && k != "snapshot") {
						trajectoryError(w, trajectory.ErrInvalid)
						return
					}
				}
				q := trajectory.Query{From: v.Get("from"), To: v.Get("to"), DeviceID: v.Get("device_id"), Page: 1}
				var e error
				if v.Get("page") != "" {
					q.Page, e = strconv.Atoi(v.Get("page"))
					if e != nil {
						trajectoryError(w, trajectory.ErrInvalid)
						return
					}
				}
				if v.Get("snapshot") != "" {
					q.Snapshot, e = time.Parse(time.RFC3339Nano, v.Get("snapshot"))
					if e != nil {
						trajectoryError(w, trajectory.ErrInvalid)
						return
					}
				}
				if _, _, e = q.Bounds(); e != nil {
					trajectoryError(w, e)
					return
				}
				u, _ := currentUser(r)
				result, e := s.Report(r.Context(), u.ID, q)
				if e != nil {
					trajectoryError(w, e)
					return
				}
				writeJSON(w, 200, result)
			})
		})
	})
}
