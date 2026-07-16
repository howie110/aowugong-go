package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
)

type authHandlers struct {
	service *auth.Service
}

// registerAuthRoutes 注册登录、注册和资料接口。
// 输入：router 是 API 路由器，service 是认证服务。
// 输出：无。
// 副作用：修改路由注册表。
func registerAuthRoutes(router chi.Router, service *auth.Service) {
	// 1. 登录与注册公开，资料接口要求 Bearer 认证。
	handlers := authHandlers{service: service}
	router.Post("/api/v1/auth/login", handlers.login)
	router.Post("/api/v1/auth/register", handlers.register)
	router.With(authenticate(service)).Get("/api/v1/auth/profile", handlers.profile)
}

// login 按 OAuth2 密码表单校验用户并返回令牌。
// 输入：request 包含 application/x-www-form-urlencoded 的 username 和 password。
// 输出：成功写入 TokenResponse。
// 副作用：读取 MySQL 并写入 HTTP 响应。
func (h authHandlers) login(w http.ResponseWriter, request *http.Request) {
	// 1. 限制并解析 URL 编码登录表单。
	request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
	if err := request.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "登录表单无效")
		return
	}

	// 2. 调用认证服务并统一转换业务错误。
	response, err := h.service.Login(request.Context(), auth.LoginRequest{
		Username: request.FormValue("username"),
		Password: request.FormValue("password"),
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// register 创建公开注册用户并返回令牌。
// 输入：request 包含 JSON username 和 password。
// 输出：成功写入 TokenResponse。
// 副作用：写入 MySQL 和 HTTP 响应。
func (h authHandlers) register(w http.ResponseWriter, request *http.Request) {
	// 1. 解码大小受限的 JSON 注册请求。
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "注册请求无效")
		return
	}

	// 2. 调用注册服务并返回与登录一致的令牌结构。
	response, err := h.service.Register(request.Context(), auth.RegisterRequest(payload))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// profile 返回当前认证用户的角色和权限资料。
// 输入：request 必须已通过 authenticate 中间件。
// 输出：成功写入 Profile。
// 副作用：读取 MySQL 和写入 HTTP 响应。
func (h authHandlers) profile(w http.ResponseWriter, request *http.Request) {
	// 1. 读取认证中间件提供的用户主键。
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}

	// 2. 查询完整资料并转换业务错误。
	profile, err := h.service.Profile(request.Context(), user.ID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

// decodeJSON 解码大小受限且只含单个 JSON 值的请求体。
// 输入：w 用于限制请求体，request 是 HTTP 请求，target 是目标结构指针。
// 输出：成功返回 nil。
// 副作用：读取并关闭请求体。
func decodeJSON(w http.ResponseWriter, request *http.Request, target any) error {
	// 1. 限制请求体并拒绝未知字段。
	request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
	defer request.Body.Close()
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	// 2. 要求请求体中只有一个 JSON 值。
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("请求体必须只包含一个 JSON 值")
	}
	return nil
}

// writeDomainError 把认证和权限业务错误统一转换为 HTTP 状态。
// 输入：err 是 service 返回的带上下文错误。
// 输出：无。
// 副作用：写入 HTTP 错误响应。
func writeDomainError(w http.ResponseWriter, err error) {
	// 1. 按可追踪业务错误映射稳定状态码。
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "请求参数无效")
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusBadRequest, "conflict", "用户名已存在")
	case errors.Is(err, auth.ErrUnauthorized), errors.Is(err, auth.ErrInvalidToken):
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "用户不存在")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
	}
}
