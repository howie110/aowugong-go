package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
	"github.com/howiedata/aowugong-go/internal/vpn"
)

type vpnHandlers struct {
	service *vpn.Service
}

// registerVPNRoutes 注册管理员 VPN 订阅资源和设备接口。
// 输入：router 是 API 路由，authService、rbacService 控制访问，service 提供业务能力。
// 输出：无。
// 副作用：修改路由注册表。
func registerVPNRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *vpn.Service) {
	// 1. 给全部接口安装登录和管理员页面权限。
	handlers := vpnHandlers{service: service}
	router.Get("/api/v1/vpn/subscriptions/{deviceID}/{token}/{format}", handlers.subscription)
	router.Route("/api/v1/vpn", func(routes chi.Router) {
		routes.Use(authenticate(authService))
		routes.Use(requirePermission(rbacService, rbac.PermissionVPN))
		routes.Get("/summary", handlers.summary)
		routes.Post("/devices", handlers.create)
		routes.Post("/devices/{deviceID}/publish", handlers.publish)
		routes.Post("/devices/{deviceID}/rotate", handlers.rotate)
		routes.Delete("/devices/{deviceID}", handlers.revoke)
		routes.Get("/devices/{deviceID}/qr", handlers.qrCode)
	})
}

// subscription 返回无需登录但受设备随机密钥保护的订阅正文。
// 输入：request 路径包含设备主键、Token 和客户端格式。
// 输出：成功写入对应配置文件，验证失败统一返回 404。
// 副作用：读取 PostgreSQL 和 VPN 私有配置文件。
func (h vpnHandlers) subscription(w http.ResponseWriter, request *http.Request) {
	// 1. 严格解析设备主键并交给服务层验证密钥和状态。
	deviceID, err := strconv.ParseInt(chi.URLParam(request, "deviceID"), 10, 64)
	if err != nil || deviceID <= 0 {
		http.NotFound(w, request)
		return
	}
	config, err := h.service.Subscription(
		request.Context(), deviceID, chi.URLParam(request, "token"), chi.URLParam(request, "format"),
	)
	if err != nil {
		http.NotFound(w, request)
		return
	}

	// 2. 禁止缓存敏感配置并以内联文件响应。
	w.Header().Set("Content-Type", config.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+config.Filename+`"`)
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(config.Body))
}

// summary 返回 VPN 页面资源、设备和分发状态。
// 输入：request 包含管理员上下文。
// 输出：成功写入 vpn.Summary JSON。
// 副作用：读取 PostgreSQL 和 VPN 私有目录。
func (h vpnHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 调用统一服务并隐藏内部错误细节。
	summary, err := h.service.Summary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取 VPN 订阅状态失败")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// create 新增并发布一台 VPN 订阅设备。
// 输入：request JSON 包含设备名和资源编码。
// 输出：成功写入新设备 JSON。
// 副作用：读取私有配置并写 PostgreSQL。
func (h vpnHandlers) create(w http.ResponseWriter, request *http.Request) {
	// 1. 限制并解析 JSON 请求体。
	var payload vpn.CreateRequest
	if err := decodeVPNJSON(request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "VPN 设备参数无效")
		return
	}
	device, err := h.service.Create(request.Context(), payload)
	if err != nil {
		writeVPNError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, device)
}

// publish 重新推送设备当前订阅配置。
// 输入：request 路径包含设备主键。
// 输出：成功写入更新后设备 JSON。
// 副作用：读取私有配置并写 PostgreSQL。
func (h vpnHandlers) publish(w http.ResponseWriter, request *http.Request) {
	// 1. 解析主键并调用统一重新发布入口。
	deviceID, ok := parseVPNDeviceID(w, request)
	if !ok {
		return
	}
	device, err := h.service.Publish(request.Context(), deviceID)
	if err != nil {
		writeVPNError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// rotate 轮换设备订阅地址并撤销旧地址。
// 输入：request 路径包含设备主键。
// 输出：成功写入新版本设备 JSON。
// 副作用：读写 PostgreSQL 并轮换设备 Token。
func (h vpnHandlers) rotate(w http.ResponseWriter, request *http.Request) {
	// 1. 解析主键并执行先发布后撤销的轮换流程。
	deviceID, ok := parseVPNDeviceID(w, request)
	if !ok {
		return
	}
	device, err := h.service.Rotate(request.Context(), deviceID)
	if err != nil {
		writeVPNError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// revoke 撤销设备当前订阅地址。
// 输入：request 路径包含设备主键。
// 输出：成功写入已撤销设备 JSON。
// 副作用：删除 Worker KV 并写 PostgreSQL。
func (h vpnHandlers) revoke(w http.ResponseWriter, request *http.Request) {
	// 1. 解析主键并执行远端优先撤销。
	deviceID, ok := parseVPNDeviceID(w, request)
	if !ok {
		return
	}
	device, err := h.service.Revoke(request.Context(), deviceID)
	if err != nil {
		writeVPNError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// qrCode 返回设备指定格式订阅地址二维码。
// 输入：request 路径包含设备主键，format 查询参数指定客户端格式。
// 输出：成功写入 PNG 图片。
// 副作用：读取 PostgreSQL 和 VPN 私有目录。
func (h vpnHandlers) qrCode(w http.ResponseWriter, request *http.Request) {
	// 1. 解析主键和格式后生成内存二维码。
	deviceID, ok := parseVPNDeviceID(w, request)
	if !ok {
		return
	}
	image, err := h.service.QRCode(request.Context(), deviceID, request.URL.Query().Get("format"))
	if err != nil {
		writeVPNError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}

// decodeVPNJSON 读取大小受限且只允许单个对象的 JSON。
// 输入：request 是 HTTP 请求，target 接收结构化数据。
// 输出：格式无效或存在额外 JSON 时返回错误。
// 副作用：读取请求体。
func decodeVPNJSON(request *http.Request, target any) error {
	// 1. 限制请求体并拒绝未知字段。
	decoder := json.NewDecoder(io.LimitReader(request.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("请求只能包含一个 JSON 对象")
	}
	return nil
}

// parseVPNDeviceID 解析并校验路由中的设备主键。
// 输入：w 用于错误响应，request 包含 chi 路由参数。
// 输出：返回正整数主键和成功标记。
// 副作用：参数无效时写入 HTTP 400。
func parseVPNDeviceID(w http.ResponseWriter, request *http.Request) (int64, bool) {
	// 1. 只接受正十进制整数。
	deviceID, err := strconv.ParseInt(chi.URLParam(request, "deviceID"), 10, 64)
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "VPN 设备编号无效")
		return 0, false
	}
	return deviceID, true
}

// writeVPNError 把 VPN 业务错误转换为稳定 HTTP 状态。
// 输入：w 是响应器，err 是服务层错误。
// 输出：无。
// 副作用：写入统一 JSON 错误响应。
func writeVPNError(w http.ResponseWriter, err error) {
	// 1. 区分参数、冲突、不存在、未配置和远端执行错误。
	switch {
	case errors.Is(err, vpn.ErrInvalidInput), errors.Is(err, vpn.ErrProfileNotFound), errors.Is(err, vpn.ErrFormatNotFound):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, vpn.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, vpn.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, vpn.ErrDistributorNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "not_configured", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "distribution_failed", "VPN 订阅分发失败")
	}
}
