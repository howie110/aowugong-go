package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/databaseview"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type databaseHandlers struct {
	service databaseReadService
}

// databaseReadService 定义数据库只读页面需要的最小查询能力。
// 输入：各方法接收请求上下文、表名、搜索条件和分页参数。
// 输出：返回数据库概况、分页数据或 CSV 导出错误。
// 副作用：只读 PostgreSQL；ExportCSV 还会向输出流写入内容。
type databaseReadService interface {
	Summary(context.Context) (databaseview.Summary, error)
	Rows(context.Context, string, string, int, int) (databaseview.RowsPage, error)
	ExportCSV(context.Context, string, string, io.Writer) error
}

type databaseExportWriter struct {
	http.ResponseWriter
	wrote bool
}

// Write 记录 CSV 是否已经开始发送，并把内容交给原始响应。
// 输入：content 是下一段 CSV 字节。
// 输出：返回底层响应写入数量和错误。
// 副作用：写 HTTP 响应并把 wrote 标记为 true。
func (w *databaseExportWriter) Write(content []byte) (int, error) {
	// 1. 首次写入前标记响应已经不能再改成 JSON 错误。
	w.wrote = true
	return w.ResponseWriter.Write(content)
}

// registerDatabaseRoutes 注册管理员只读数据库概况、分页和导出接口。
// 输入：router 是 API 路由器，认证权限服务负责保护入口，service 只允许白名单查询。
// 输出：无。
// 副作用：修改路由注册表。
func registerDatabaseRoutes(
	router chi.Router,
	authService *auth.Service,
	rbacService *rbac.Service,
	service databaseReadService,
) {
	// 1. 全部数据库接口统一要求登录和数据库页面权限。
	handlers := databaseHandlers{service: service}
	router.Route("/api/v1/database", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionDatabase))
		routes.Get("/summary", handlers.summary)
		routes.Get("/tables/{table}", handlers.rows)
		routes.Get("/tables/{table}/export", handlers.export)
	})
}

// summary 返回 PostgreSQL 数据库和应用表概况。
// 输入：request 提供查询上下文。
// 输出：成功写入数据库概况 JSON，失败写入统一错误。
// 副作用：只读 PostgreSQL 并写入 HTTP 响应。
func (h databaseHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 调用只读服务并统一隐藏底层错误细节。
	summary, err := h.service.Summary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取数据库概况失败")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// rows 返回指定 PostgreSQL 表的一页数据。
// 输入：路径提供表名，查询参数提供 page、page_size 和 search。
// 输出：成功写入分页 JSON，参数或表错误写入统一错误。
// 副作用：只读 PostgreSQL 并写入 HTTP 响应。
func (h databaseHandlers) rows(w http.ResponseWriter, request *http.Request) {
	// 1. 解析有上限的分页参数。
	page, err := parseDatabasePositiveInt(request.URL.Query().Get("page"), 1, 1, 1000000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "页码参数无效")
		return
	}
	pageSize, err := parseDatabasePositiveInt(request.URL.Query().Get("page_size"), 50, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "每页数量参数无效")
		return
	}

	// 2. 查询白名单表并区分不存在与内部错误。
	result, err := h.service.Rows(
		request.Context(), chi.URLParam(request, "table"),
		request.URL.Query().Get("search"), page, pageSize,
	)
	if errors.Is(err, databaseview.ErrTableNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "数据表不存在")
		return
	}
	if errors.Is(err, databaseview.ErrInvalidPagination) ||
		errors.Is(err, databaseview.ErrSearchTooLong) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取数据库表失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// export 导出指定 PostgreSQL 表的脱敏 CSV。
// 输入：路径提供表名，search 可限制导出内容。
// 输出：成功写入 CSV 文件，表、规模或查询失败写入统一错误。
// 副作用：只读 PostgreSQL，并流式写入 HTTP 响应。
func (h databaseHandlers) export(w http.ResponseWriter, request *http.Request) {
	// 1. 调用只读导出入口并在写响应头前处理全部错误。
	table := chi.URLParam(request, "table")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	fileName := strings.Map(func(value rune) rune {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9' || value == '_' || value == '-' {
			return value
		}
		return '_'
	}, table)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, fileName))
	stream := &databaseExportWriter{ResponseWriter: w}
	err := h.service.ExportCSV(
		request.Context(), table, request.URL.Query().Get("search"), stream,
	)
	if stream.wrote {
		return
	}
	w.Header().Del("Content-Disposition")
	if errors.Is(err, databaseview.ErrTableNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "数据表不存在")
		return
	}
	if errors.Is(err, databaseview.ErrExportTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "export_too_large", "导出结果超过十万行，请缩小范围")
		return
	}
	if errors.Is(err, databaseview.ErrSearchTooLong) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "导出数据库表失败")
		return
	}
}

// parseDatabasePositiveInt 解析数据库页面使用的有界正整数。
// 输入：value 是查询文本，fallback 是空值默认数，min 和 max 是闭区间。
// 输出：返回有效整数；格式或范围错误时返回错误。
// 副作用：无。
func parseDatabasePositiveInt(value string, fallback, min, max int) (int, error) {
	// 1. 空值使用调用方明确提供的默认数。
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	// 2. 解析十进制并验证闭区间。
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		return 0, fmt.Errorf("整数必须在 %d 到 %d 之间", min, max)
	}
	return parsed, nil
}
