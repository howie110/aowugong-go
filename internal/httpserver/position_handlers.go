package httpserver

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/finance/position"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

const maxPositionRequestBytes = 100 << 20

type positionHandlers struct {
	service *position.Service
}

// registerPositionRoutes 注册仓位摘要、最近记录和截图上传接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供仓位业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerPositionRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *position.Service) {
	// 1. 给全部仓位接口安装认证和页面权限中间件。
	handlers := positionHandlers{service: service}
	router.Route("/api/v1/finance/positions", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionFinancePositions))
		routes.Get("/summary", handlers.summary)
		routes.Get("/snapshots/recent", handlers.recent)
		routes.Post("/snapshots/upload", handlers.upload)
	})
}

// summary 返回股票仓位导入页面摘要。
// 输入：request 已通过认证和仓位权限校验。
// 输出：写入摘要 JSON。
// 副作用：读取 MySQL 并写入 HTTP 响应。
func (h positionHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 读取统一仓位摘要并转换服务错误。
	result, err := h.service.Summary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取仓位导入摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// recent 返回最近股票仓位导入记录。
// 输入：查询参数 limit 可选，范围 1 到 100。
// 输出：写入 Snapshot 数组。
// 副作用：读取 MySQL 并写入 HTTP 响应。
func (h positionHandlers) recent(w http.ResponseWriter, request *http.Request) {
	// 1. 解析并约束记录上限。
	limit := 20
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit 必须在 1 到 100 之间")
			return
		}
		limit = parsed
	}

	// 2. 读取并返回最近记录。
	results, err := h.service.Recent(request.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取最近仓位记录失败")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// upload 读取一批 multipart 截图并调用统一仓位处理服务。
// 输入：表单包含 files、snapshot_date、broker_name 和 source_app。
// 输出：写入每个文件的成功或失败结果。
// 副作用：读取请求体、写入文件和 MySQL，并调用阿里云 OCR。
func (h positionHandlers) upload(w http.ResponseWriter, request *http.Request) {
	// 1. 限制总请求大小并解析 multipart 表单。
	request.Body = http.MaxBytesReader(w, request.Body, maxPositionRequestBytes)
	if err := request.ParseMultipartForm(maxPositionRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "仓位截图请求无效或超过 100MB")
		return
	}
	headers := request.MultipartForm.File["files"]
	if len(headers) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "缺少仓位截图")
		return
	}
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}

	// 2. 读取每个文件，读取错误作为对应文件失败结果保留。
	files := make([]position.Upload, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			files = append(files, position.Upload{Filename: header.Filename})
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxPositionRequestBytes+1))
		_ = file.Close()
		if readErr != nil {
			files = append(files, position.Upload{Filename: header.Filename})
			continue
		}
		files = append(files, position.Upload{Filename: header.Filename, Data: data})
	}

	// 3. 通过统一批量入口处理并返回 200，单文件错误位于 results。
	response := h.service.ProcessBatch(request.Context(), position.BatchRequest{
		SnapshotDate: request.FormValue("snapshot_date"),
		BrokerName:   request.FormValue("broker_name"),
		SourceApp:    request.FormValue("source_app"),
		CreatedBy:    user.Username,
		Files:        files,
	})
	writeJSON(w, http.StatusOK, response)
}
