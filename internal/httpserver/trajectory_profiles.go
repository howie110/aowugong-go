package httpserver

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/trajectory"
)

const trajectoryProfileTTL = 15 * time.Minute

type trajectoryProfile struct {
	deviceID, body string
	expires        time.Time
	timer          *time.Timer
}

// Profiles are short-lived secrets in process memory; the database keeps only
// the upload token hash. Restarting the server invalidates outstanding links.
type trajectoryProfiles struct {
	mu      sync.Mutex
	entries map[string]trajectoryProfile
	origin  string
}

func newTrajectoryProfiles(origin string) *trajectoryProfiles {
	return &trajectoryProfiles{entries: make(map[string]trajectoryProfile), origin: strings.TrimRight(origin, "/")}
}

func (p *trajectoryProfiles) issue(c trajectory.Credential, now time.Time) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, entry := range p.entries {
		if entry.deviceID == c.Device.ID || !entry.expires.After(now) {
			entry.timer.Stop()
			delete(p.entries, key)
		}
	}
	// Bound temporary secret storage even when many accounts issue links.
	if len(p.entries) >= 256 {
		oldest := ""
		var expiry time.Time
		for key, entry := range p.entries {
			if oldest == "" || entry.expires.Before(expiry) {
				oldest = key
				expiry = entry.expires
			}
		}
		p.entries[oldest].timer.Stop()
		delete(p.entries, oldest)
	}
	ticket := rand.Text()
	expires := now.Add(trajectoryProfileTTL)
	entry := trajectoryProfile{deviceID: c.Device.ID, body: gpsLoggerProfile(p.origin, c.Token), expires: expires}
	entry.timer = time.AfterFunc(time.Until(expires), func() { p.mu.Lock(); defer p.mu.Unlock(); delete(p.entries, ticket) })
	p.entries[ticket] = entry
	return "/api/v1/trajectory/profiles/" + ticket + "/aowugong.properties"
}

func (p *trajectoryProfiles) revoke(deviceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, entry := range p.entries {
		if entry.deviceID == deviceID {
			entry.timer.Stop()
			delete(p.entries, key)
		}
	}
}

func (p *trajectoryProfiles) download(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	p.mu.Lock()
	entry, ok := p.entries[chi.URLParam(r, "ticket")]
	p.mu.Unlock()
	if !ok || !entry.expires.After(time.Now()) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="aowugong.properties"`)
	_, _ = w.Write([]byte(entry.body))
}

func gpsLoggerProfile(origin, token string) string {
	// Java Properties decodes escaped newlines inside the HTTP header value.
	escape := strings.NewReplacer("\\", "\\\\", "\r", "\\r", "\n", "\\n")
	return fmt.Sprintf(`log_customurl_enabled=true
log_customurl_url=%s/api/v1/trajectory/collect
log_customurl_method=POST
log_customurl_headers=Content-Type: application/x-www-form-urlencoded\nAuthorization: Bearer %s
log_customurl_body=timestamp=%%TIMESTAMP&lat=%%LAT&lon=%%LON&acc=%%ACC&alt=%%ALT&spd=%%SPD&dir=%%DIR&prov=%%PROV
log_customurl_basicauth_username=
log_customurl_basicauth_password=
log_customurl_discard_offline_locations_enabled=false
time_before_logging=60
distance_before_logging=0
log_plain_text=true
debugtofile=false
`, escape.Replace(origin), escape.Replace(token))
}
