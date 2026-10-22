package httpserver

import (
	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/trajectory"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTrajectoryProfileImport(t *testing.T) {
	now := time.Now()
	p := newTrajectoryProfiles("https://example.com/")
	c := trajectory.Credential{Device: trajectory.Device{ID: "device-1"}, Token: strings.Repeat("a", 64)}
	link := p.issue(c, now)
	if strings.Contains(link, c.Token) {
		t.Fatal("upload token exposed in URL")
	}
	r := chi.NewRouter()
	r.Get("/api/v1/trajectory/profiles/{ticket}/aowugong.properties", p.download)
	fetch := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := fetch(link)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, line := range []string{"log_customurl_enabled=true", "log_customurl_url=https://example.com/api/v1/trajectory/collect", "log_customurl_method=POST", `log_customurl_headers=Content-Type: application/x-www-form-urlencoded\nAuthorization: Bearer ` + c.Token, "timestamp=%TIMESTAMP", "time_before_logging=60", "log_plain_text=true", "log_customurl_discard_offline_locations_enabled=false"} {
		if !strings.Contains(w.Body.String(), line) {
			t.Fatalf("missing profile setting %q", line)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("profile must not be cached")
	}
	if fetch(link).Code != 200 {
		t.Fatal("downloads must tolerate client retries")
	}
	next := p.issue(c, now)
	if fetch(link).Code != 404 || fetch(next).Code != 200 {
		t.Fatal("rotation must invalidate earlier links")
	}
	p.revoke(c.Device.ID)
	if fetch(next).Code != 404 {
		t.Fatal("revoked device profile still available")
	}
	expired := p.issue(c, now.Add(-16*time.Minute))
	if fetch(expired).Code != 404 {
		t.Fatal("expired profile available")
	}
	if fetch("/api/v1/trajectory/profiles/unknown/aowugong.properties").Code != 404 {
		t.Fatal("unknown profile available")
	}
}
