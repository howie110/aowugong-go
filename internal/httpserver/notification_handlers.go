package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/howiedata/aowugong-go/internal/notification"
)

// newNotificationHandler 供服务器或本地脚本通过 HTTPS 域名调用，来源由专用 Token 决定。
func newNotificationHandler(service *notification.Service, tokens map[string]string) http.Handler {
	credentials := notificationCredentials(tokens)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		project := authenticateNotification(r, credentials)
		if project == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, 401, "invalid_notification_token", "缺少或无效的项目通知 Token")
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, 415, "unsupported_media_type", "需要 application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		defer r.Body.Close()
		// 仅限制此小型 JSON 接口的读取时间，不改变其他上传接口的超时。
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
		defer controller.SetReadDeadline(time.Time{})
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input struct {
			RequestID string `json:"request_id"`
			Title     string `json:"title"`
			Content   string `json:"content"`
		}
		decodeErr := decoder.Decode(&input)
		if decodeErr == nil {
			var extra any
			decodeErr = decoder.Decode(&extra)
			if errors.Is(decodeErr, io.EOF) {
				decodeErr = nil
			} else if decodeErr == nil {
				decodeErr = errors.New("multiple JSON values")
			}
		}
		if decodeErr != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(decodeErr, &tooLarge) {
				writeError(w, 413, "payload_too_large", "请求不得超过 16 KiB")
			} else {
				writeError(w, 400, "invalid_json", "需要单个合法 JSON 对象，且不能包含未知字段")
			}
			return
		}
		result, err := service.Dispatch(r.Context(), notification.DispatchInput{Source: project, RequestID: input.RequestID, Title: input.Title, Content: input.Content})
		if err != nil {
			switch {
			case errors.Is(err, notification.ErrInvalid):
				writeError(w, 400, "invalid_notification", err.Error())
			case errors.Is(err, notification.ErrConflict):
				writeError(w, 409, "idempotency_conflict", err.Error())
			case errors.Is(err, notification.ErrRateLimited):
				w.Header().Set("Retry-After", "60")
				writeError(w, 429, "rate_limited", err.Error())
			default:
				writeError(w, 503, "notification_unavailable", notification.ErrStorage.Error())
			}
			return
		}
		status := http.StatusOK
		if result.Status != "success" {
			status = http.StatusAccepted
		}
		writeJSON(w, status, result)
	})
}
