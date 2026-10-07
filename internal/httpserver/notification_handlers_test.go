package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/notification"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type notificationSender struct {
	calls int
	err   error
}

func (s *notificationSender) SendText(context.Context, string) error { s.calls++; return s.err }

const testNotificationToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const secondNotificationToken = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func notificationTestRouter(service *notification.Service, tokens map[string]string) http.Handler {
	return NewRouter(Dependencies{Notification: service, NotificationTokens: tokens})
}

func notificationRequest(h http.Handler, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://aowugong.top/api/v1/notifications/wechat", strings.NewReader(body))
	r.RemoteAddr = "203.0.113.2:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.2")
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestNotificationProjectAuthentication(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &notificationSender{}
	service := notification.NewService(notification.NewRepository(db), sender)
	tokens := map[string]string{"receipt-split": testNotificationToken, "other-project": secondNotificationToken}
	h := notificationTestRouter(service, tokens)
	body := `{"request_id":"one","title":"完成","content":"已处理"}`
	for _, token := range []string{"", "wrong", testNotificationToken + "x"} {
		w := notificationRequest(h, token, body)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("want 401: %d %s", w.Code, w.Body)
		}
	}
	if sender.calls != 0 {
		t.Fatal("unauthenticated request sent notification")
	}
	for _, token := range []string{testNotificationToken, secondNotificationToken} {
		w := notificationRequest(h, token, body)
		if w.Code != 200 {
			t.Fatalf("authenticated domain request: %d %s", w.Code, w.Body)
		}
	}
	if sender.calls != 2 {
		t.Fatalf("projects must have independent idempotency namespaces: %d", sender.calls)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_log WHERE source IN ('receipt-split','other-project') AND request_id='one'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("source count=%d err=%v", count, err)
	}
	w := notificationRequest(h, testNotificationToken, `{"source":"other-project","request_id":"spoof","title":"test","content":"body"}`)
	if w.Code != 400 || sender.calls != 2 {
		t.Fatalf("spoof accepted: %d", w.Code)
	}
	delete(tokens, "receipt-split")
	h = notificationTestRouter(service, tokens)
	if w = notificationRequest(h, testNotificationToken, body); w.Code != 401 {
		t.Fatalf("revoked token accepted: %d", w.Code)
	}
	if w = notificationRequest(h, secondNotificationToken, body); w.Code != 200 {
		t.Fatalf("other project broken: %d", w.Code)
	}
	for _, deps := range []Dependencies{{}, {Notification: service}} {
		if w = notificationRequest(NewRouter(deps), testNotificationToken, body); w.Code != 404 {
			t.Fatalf("unconfigured endpoint exposed: %d", w.Code)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/internal/notifications/wechat", strings.NewReader(body)))
	if w.Code != 404 {
		t.Fatalf("old endpoint remains: %d", w.Code)
	}
}

func TestNotificationRejectsMalformedRequests(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &notificationSender{}
	h := notificationTestRouter(notification.NewService(notification.NewRepository(db), sender), map[string]string{"test": testNotificationToken})
	for _, tc := range []struct {
		method, contentType, body string
		want                      int
	}{
		{"GET", "application/json", "{}", 405},
		{"POST", "text/plain", "{}", 415},
		{"POST", "application/json", "{}", 400},
		{"POST", "application/json", `{"unknown":1}`, 400},
		{"POST", "application/json", `{} {}`, 400},
		{"POST", "application/json", strings.Repeat(" ", 17000) + "{}", 413},
	} {
		r := httptest.NewRequest(tc.method, "https://aowugong.top/api/v1/notifications/wechat", strings.NewReader(tc.body))
		r.RemoteAddr = "127.0.0.1:1000"
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("Authorization", "Bearer "+testNotificationToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("want=%d got=%d body=%s", tc.want, w.Code, w.Body)
		}
	}
	if sender.calls != 0 {
		t.Fatal("invalid request sent a notification")
	}
}

func TestNotificationResponseContract(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &notificationSender{}
	h := notificationTestRouter(notification.NewService(notification.NewRepository(db), sender), map[string]string{"test": testNotificationToken})
	call := func(key, body string) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{"request_id": key, "title": "test", "content": body})
		r := httptest.NewRequest("POST", "https://aowugong.top/api/v1/notifications/wechat", strings.NewReader(string(payload)))
		r.RemoteAddr = "127.0.0.1:1000"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testNotificationToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i := range 10 {
		w := call(fmt.Sprintf("key-%d", i), "body")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	w := call("key-0", "body")
	var result notification.DispatchResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !result.Duplicate || result.Status != "success" || sender.calls != 10 {
		t.Fatalf("repeat: %d %+v sends=%d", w.Code, result, sender.calls)
	}
	if w = call("key-0", "changed"); w.Code != 409 {
		t.Fatalf("conflict: %d", w.Code)
	}
	if w = call("over-limit", "body"); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("limit: %d", w.Code)
	}
	// 新服务仅重置内存限流，不清除持久化去重记录。
	sender.err = errors.New("upstream key=SECRET")
	h = notificationTestRouter(notification.NewService(notification.NewRepository(db), sender), map[string]string{"test": testNotificationToken})
	if w = call("timeout", "body"); w.Code != 202 || !strings.Contains(w.Body.String(), `"status":"unknown"`) || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatalf("unknown: %d %s", w.Code, w.Body)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if w = call("storage-failure", "body"); w.Code != 503 {
		t.Fatalf("storage: %d", w.Code)
	}
}

func TestNotificationOnlyAcceptsSingleBearerHeader(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &notificationSender{}
	h := notificationTestRouter(notification.NewService(notification.NewRepository(db), sender), map[string]string{"test": testNotificationToken})
	for _, headers := range [][]string{nil, {"Basic " + testNotificationToken}, {"Bearer"}, {"Bearer " + testNotificationToken + " extra"}, {"Bearer " + testNotificationToken, "Bearer " + testNotificationToken}} {
		r := httptest.NewRequest("POST", "https://aowugong.top/api/v1/notifications/wechat?token="+testNotificationToken, strings.NewReader(`{"request_id":"one","title":"test","content":"body"}`))
		for _, header := range headers {
			r.Header.Add("Authorization", header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 || strings.Contains(w.Body.String(), testNotificationToken) {
			t.Fatalf("invalid header accepted or token leaked: %d", w.Code)
		}
	}
	if sender.calls != 0 {
		t.Fatal("malformed authentication sent a notification")
	}
}
