package httpserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type currentUserContextKey struct{}

// authenticate 要求有效 Bearer 令牌并把当前用户写入请求上下文。
// 输入：service 是认证服务，next 是受保护处理器。
// 输出：返回认证中间件。
// 副作用：读取 MySQL，失败或成功时写入 HTTP 响应或请求上下文。
func authenticate(service *auth.Service) func(http.Handler) http.Handler {
	// 1. 返回逐请求解析 Authorization 头的中间件。
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			// 2. 严格解析 Bearer 方案及非空令牌。
			parts := strings.Fields(request.Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, http.StatusUnauthorized, "unauthorized", "认证令牌无效或已过期")
				return
			}

			// 3. 校验令牌和用户实时状态，并继续处理请求。
			user, err := service.Authenticate(request.Context(), parts[1])
			if err != nil {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, http.StatusUnauthorized, "unauthorized", "认证令牌无效或已过期")
				return
			}
			ctx := context.WithValue(request.Context(), currentUserContextKey{}, user)
			next.ServeHTTP(w, request.WithContext(ctx))
		})
	}
}

// requirePermission 要求当前用户拥有指定页面权限。
// 输入：service 是 RBAC 服务，permissionCode 是权限编码。
// 输出：返回权限中间件。
// 副作用：读取 MySQL，失败时写入 HTTP 响应。
func requirePermission(service *rbac.Service, permissionCode string) func(http.Handler) http.Handler {
	// 1. 返回基于已认证用户主键检查权限的中间件。
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			// 2. 从认证中间件写入的上下文读取当前用户。
			user, ok := request.Context().Value(currentUserContextKey{}).(auth.User)
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
				return
			}

			// 3. 查询权限并区分服务错误与拒绝访问。
			allowed, err := service.HasPermission(request.Context(), user.ID, permissionCode)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "读取用户权限失败")
				return
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "forbidden", "无权访问此功能")
				return
			}
			next.ServeHTTP(w, request)
		})
	}
}

// currentUser 返回认证中间件写入请求上下文的用户。
// 输入：request 是已通过认证的请求。
// 输出：返回当前用户及存在标记。
// 副作用：无。
func currentUser(request *http.Request) (auth.User, bool) {
	// 1. 使用私有键读取并断言用户类型。
	user, ok := request.Context().Value(currentUserContextKey{}).(auth.User)
	return user, ok
}
