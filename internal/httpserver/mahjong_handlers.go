package httpserver

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/mahjong"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

const maxMahjongUploadBytes = 20 << 20

type mahjongHandlers struct {
	service *mahjong.Service
}

// registerMahjongRoutes 注册麻将摘要、报告、记录和 Excel 导入接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供业务能力。
// 输出：无。
// 副作用：修改路由注册表。
func registerMahjongRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *mahjong.Service) {
	// 1. 给全部麻将接口安装认证和页面权限中间件。
	handlers := mahjongHandlers{service: service}
	router.Route("/api/v1/mahjong", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionMahjong))
		routes.Get("/summary", handlers.summary)
		routes.Get("/report", handlers.report)
		routes.Get("/records/recent", handlers.recent)
		routes.Post("/records/save", handlers.save)
		routes.Post("/records/import", handlers.importExcel)
	})
}

// summary 返回控制台使用的麻将摘要。
// 输入：request 已通过认证和麻将权限校验。
// 输出：写入摘要 JSON。
// 副作用：读取 MySQL 和写入 HTTP 响应。
func (h mahjongHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 复用麻将服务的统一统计口径。
	summary, err := h.service.PageSummary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取麻将摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// report 返回麻将完整统计报告。
// 输入：查询参数 limit 和 table_fee 可选。
// 输出：写入 Report。
// 副作用：读取 MySQL 和写入 HTTP 响应。
func (h mahjongHandlers) report(w http.ResponseWriter, request *http.Request) {
	// 1. 解析并约束可选查询参数。
	limit := 1000
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 5000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit 必须在 1 到 5000 之间")
			return
		}
		limit = parsed
	}
	tableFee := request.URL.Query().Get("table_fee")
	if tableFee == "" {
		tableFee = "9"
	}

	// 2. 生成报告并转换无效场费错误。
	report, err := h.service.Report(request.Context(), limit, tableFee)
	if err != nil {
		if errors.Is(err, mahjong.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid_request", "table_fee 必须是数字")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "读取麻将报告失败")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// recent 返回最近麻将战绩。
// 输入：查询参数 limit 可选。
// 输出：写入 Record 数组。
// 副作用：读取 MySQL 和写入 HTTP 响应。
func (h mahjongHandlers) recent(w http.ResponseWriter, request *http.Request) {
	// 1. 解析最近记录上限。
	limit := 30
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit 必须在 1 到 200 之间")
			return
		}
		limit = parsed
	}

	// 2. 读取并返回最近记录。
	records, err := h.service.Recent(request.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取最近麻将战绩失败")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// save 保存页面录入的单日麻将战绩。
// 输入：请求体包含 played_date 和 result_amount。
// 输出：写入 WriteResponse。
// 副作用：写入 MySQL 和 HTTP 响应。
func (h mahjongHandlers) save(w http.ResponseWriter, request *http.Request) {
	// 1. 解码 JSON 并读取当前用户名。
	var payload mahjong.WriteRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "麻将战绩请求无效")
		return
	}
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}

	// 2. 调用唯一保存入口并转换参数错误。
	response, err := h.service.Save(request.Context(), payload, user.Username)
	if err != nil {
		if errors.Is(err, mahjong.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid_request", "日期或当日输赢无效")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "保存麻将战绩失败")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// importExcel 导入麻将历史 Excel 文件。
// 输入：multipart/form-data 的 file 字段必须是 .xlsx。
// 输出：写入 ImportResponse。
// 副作用：读取上传文件、写入 MySQL 和 HTTP 响应。
func (h mahjongHandlers) importExcel(w http.ResponseWriter, request *http.Request) {
	// 1. 限制请求体并读取 multipart 文件。
	request.Body = http.MaxBytesReader(w, request.Body, maxMahjongUploadBytes)
	if err := request.ParseMultipartForm(maxMahjongUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Excel 文件无效或超过 20MB")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "缺少 Excel 文件")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		writeError(w, http.StatusBadRequest, "invalid_request", "只支持 .xlsx 文件")
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, maxMahjongUploadBytes+1))
	if err != nil || len(content) > maxMahjongUploadBytes {
		writeError(w, http.StatusBadRequest, "invalid_request", "读取 Excel 文件失败")
		return
	}

	// 2. 调用统一导入服务并返回写入统计。
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}
	response, err := h.service.ImportExcel(request.Context(), content, header.Filename, user.Username)
	if err != nil {
		if errors.Is(err, mahjong.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "导入麻将 Excel 失败")
		return
	}
	writeJSON(w, http.StatusOK, response)
}
