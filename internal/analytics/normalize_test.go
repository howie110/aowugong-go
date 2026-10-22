package analytics

import (
	"testing"
	"time"
)

func TestNormalizeDoesNotRetainSecrets(t *testing.T) {
	path, address, err := NormalizeURL("/blog/post?token=secret&utm_source=newsletter&q=private#secret")
	if err != nil || path != "/blog/post" || address != "/blog/post?utm_source=newsletter" {
		t.Fatalf("%q %q %v", path, address, err)
	}
	for _, value := range []string{"https://evil.test/a", "//evil.test", "/api/v1/health", "/blog/rss.xml", "/assets/app.js"} {
		if _, _, err := NormalizeURL(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}
func TestAnonymousVisitorRotation(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	a := VisitorID("secret", "192.0.2.1", "browser", now)
	if a == "" || a != VisitorID("secret", "192.0.2.1", "browser", now.Add(time.Hour)) {
		t.Fatal("unstable identity")
	}
	if a == VisitorID("secret", "192.0.2.1", "browser", now.AddDate(0, 1, 0)) {
		t.Fatal("identity did not rotate")
	}
}
func TestReferrerAndChannel(t *testing.T) {
	if ReferrerDomain("https://aowugong.top/blog?a=private", "aowugong.top") != "" {
		t.Fatal("internal referrer")
	}
	if ReferrerDomain("https://www.google.com/search?q=secret", "aowugong.top") != "www.google.com" {
		t.Fatal("domain")
	}
	if Channel("www.google.com", "") != "search" || Channel("", "") != "direct" || Channel("evil-google.com", "") != "referral" {
		t.Fatal("classification")
	}
}
