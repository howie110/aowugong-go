package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/blog"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

func registerBlogRoutes(router chi.Router, deps Dependencies) {
	if deps.BlogArticles != nil {
		router.Get("/api/v1/blog/posts", func(w http.ResponseWriter, r *http.Request) {
			articles, err := deps.BlogArticles.List()
			if err != nil {
				blogError(w, err)
				return
			}
			for i := range articles {
				articles[i].HTML = ""
				articles[i].TOC = nil
			}
			writeJSON(w, 200, articles)
		})
		router.Get("/api/v1/blog/posts/*", func(w http.ResponseWriter, r *http.Request) {
			a, err := deps.BlogArticles.Get(chi.URLParam(r, "*"))
			if err != nil {
				blogError(w, err)
				return
			}
			writeJSON(w, 200, a)
		})
	}
	if deps.BlogStatuses == nil {
		return
	}
	list := func(publishedOnly bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			limit, err := blogInt(r, "limit", 20)
			if err != nil {
				blogError(w, blog.ErrInvalidInput)
				return
			}
			offset, err := blogInt(r, "offset", 0)
			if err != nil {
				blogError(w, blog.ErrInvalidInput)
				return
			}
			rows, err := deps.BlogStatuses.List(r.Context(), publishedOnly, limit, offset)
			if err != nil {
				blogError(w, err)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, rows)
		}
	}
	router.Get("/api/v1/blog/statuses", list(true))
	if deps.Auth == nil || deps.RBAC == nil {
		return
	}
	router.Route("/api/v1/blog/admin", func(admin chi.Router) {
		admin.Use(authenticate(deps.Auth), requirePermission(deps.RBAC, rbac.PermissionBlogStatus))
		admin.Get("/statuses", list(false))
		admin.Post("/images", func(w http.ResponseWriter, r *http.Request) {
			if deps.BlogMedia == nil {
				writeError(w, 503, "media_unavailable", "尚未配置图片上传")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, blog.MaxImageBytes+(1<<20))
			if err := r.ParseMultipartForm(blog.MaxImageBytes); err != nil {
				writeError(w, 400, "invalid_input", "图片过大或上传格式无效")
				return
			}
			if r.MultipartForm != nil {
				defer r.MultipartForm.RemoveAll()
			}
			file, header, err := r.FormFile("image")
			if err != nil {
				writeError(w, 400, "invalid_input", "缺少图片")
				return
			}
			defer file.Close()
			result, err := deps.BlogMedia.Upload(r.Context(), file, header.Header.Get("Content-Type"))
			if err != nil {
				blogError(w, err)
				return
			}
			writeJSON(w, 201, result)
		})
		save := func(create bool) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				var input blog.StatusInput
				if err := decodeJSON(w, r, &input); err != nil {
					writeError(w, 400, "invalid_input", "状态格式无效")
					return
				}
				input.Create = create
				if !create {
					if input.ID != "" && input.ID != chi.URLParam(r, "id") {
						blogError(w, blog.ErrInvalidInput)
						return
					}
					input.ID = chi.URLParam(r, "id")
				}
				user, _ := currentUser(r)
				status, err := deps.BlogStatuses.Save(r.Context(), user.ID, input)
				if err != nil {
					blogError(w, err)
					return
				}
				code := 200
				if create {
					code = 201
				}
				writeJSON(w, code, status)
			}
		}
		admin.Post("/statuses", save(true))
		admin.Put("/statuses/{id}", save(false))
		admin.Delete("/statuses/{id}", func(w http.ResponseWriter, r *http.Request) {
			if err := deps.BlogStatuses.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
				blogError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
func blogInt(r *http.Request, key string, fallback int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
func blogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blog.ErrNotFound):
		writeError(w, 404, "not_found", "内容不存在")
	case errors.Is(err, blog.ErrInvalidInput):
		writeError(w, 400, "invalid_input", err.Error())
	case errors.Is(err, blog.ErrConflict):
		writeError(w, 409, "conflict", err.Error())
	default:
		slog.Error("博客请求失败", "error", err)
		writeError(w, 500, "internal_error", "博客服务暂时不可用")
	}
}
