package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/chatarchive"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type archiveReadService interface {
	ListAudit(context.Context, chatarchive.AuditFilter) (chatarchive.AuditPage, error)
	Statistics(context.Context, int64, string) (chatarchive.Statistics, error)
	ListConversations(context.Context, chatarchive.Filter) ([]chatarchive.Conversation, error)
	ListMessages(context.Context, int64, int64, int) ([]chatarchive.Message, error)
	ListVersions(context.Context, int64, string, int64, int) ([]chatarchive.Message, error)
	Export(context.Context, int64, string, io.Writer) error
}

// registerArchiveRoutes 同时要求管理员身份和页面权限，不开放写入端点。
func registerArchiveRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service archiveReadService) {
	router.Route("/api/v1/chat-archive", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requireAdministrator(rbacService))
		routes.Use(requirePermission(rbacService, rbac.PermissionChatArchive))
		routes.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
				defer cancel()
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
		routes.Get("/statistics", func(w http.ResponseWriter, r *http.Request) {
			snapshot, err := parseArchiveCursor(r.URL.Query().Get("snapshot"))
			if err != nil {
				archiveError(w, chatarchive.ErrInvalidFilter)
				return
			}
			result, err := service.Statistics(r.Context(), snapshot, r.URL.Query().Get("before"))
			if err != nil {
				archiveError(w, err)
				return
			}
			writeJSON(w, 200, result)
		})
		routes.Get("/messages", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			limit, e1 := parseDatabasePositiveInt(q.Get("limit"), 20, 1, 200)
			offset, e2 := parseArchiveCursor(q.Get("offset"))
			snapshot, e3 := parseArchiveCursor(q.Get("snapshot"))
			f := chatarchive.AuditFilter{Source: q.Get("source"), From: q.Get("from"), To: q.Get("to"), Query: q.Get("q"), ConversationID: q.Get("conversation_id"), MessageID: q.Get("message_id"), Order: q.Get("order"), Limit: limit, Offset: int(offset), Snapshot: snapshot}
			if e1 != nil || e2 != nil || e3 != nil || offset > 1000000 || f.Validate() != nil {
				archiveError(w, chatarchive.ErrInvalidFilter)
				return
			}
			result, err := service.ListAudit(r.Context(), f)
			if err != nil {
				archiveError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
		})
		routes.Get("/conversations", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			limit, e1 := parseDatabasePositiveInt(q.Get("limit"), 20, 1, 200)
			offset, e2 := parseArchiveCursor(q.Get("offset"))
			f := chatarchive.Filter{Source: q.Get("source"), From: q.Get("from"), To: q.Get("to"), Title: q.Get("title"), Query: q.Get("q"), Limit: limit, Offset: int(offset)}
			if e1 != nil || e2 != nil || f.Validate() != nil {
				archiveError(w, chatarchive.ErrInvalidFilter)
				return
			}
			result, err := service.ListConversations(r.Context(), f)
			if err != nil {
				archiveError(w, err)
				return
			}
			writeJSON(w, 200, result)
		})
		routes.Get("/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) { archiveMessages(w, r, service, false) })
		routes.Get("/conversations/{id}/versions", func(w http.ResponseWriter, r *http.Request) { archiveMessages(w, r, service, true) })
		routes.Get("/conversations/{id}/export", func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			format := r.URL.Query().Get("format")
			if err != nil || id < 1 || (format != "raw" && format != "jsonl") {
				archiveError(w, chatarchive.ErrInvalidFilter)
				return
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Disposition", `attachment; filename="chat-archive.jsonl"`)
			tracker := &databaseExportWriter{ResponseWriter: w}
			if err := service.Export(r.Context(), id, format, tracker); err != nil {
				if !tracker.wrote {
					w.Header().Del("Content-Disposition")
					archiveError(w, err)
				} else {
					panic(http.ErrAbortHandler)
				}
			}
		})
	})
}

func archiveMessages(w http.ResponseWriter, r *http.Request, service archiveReadService, versions bool) {
	id, e1 := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	after, e2 := parseArchiveCursor(r.URL.Query().Get("after"))
	limit, e3 := parseDatabasePositiveInt(r.URL.Query().Get("limit"), 100, 1, 200)
	key := r.URL.Query().Get("message_key")
	if e1 != nil || id < 1 || e2 != nil || e3 != nil || (versions && (key == "" || len(key) > 1000)) {
		archiveError(w, chatarchive.ErrInvalidFilter)
		return
	}
	var result []chatarchive.Message
	var err error
	if versions {
		result, err = service.ListVersions(r.Context(), id, key, after, limit)
	} else {
		result, err = service.ListMessages(r.Context(), id, after, limit)
	}
	if err != nil {
		archiveError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func parseArchiveCursor(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, chatarchive.ErrInvalidFilter
	}
	return n, nil
}

func archiveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chatarchive.ErrInvalidFilter):
		writeError(w, 400, "invalid_request", "归档查询参数无效")
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "not_found", "会话不存在")
	default:
		writeError(w, 500, "internal_error", "读取聊天归档失败")
	}
}
