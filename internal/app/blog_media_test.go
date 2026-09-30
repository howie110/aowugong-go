package app

import (
	"github.com/howiedata/aowugong-go/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlogMediaUsesDedicatedIdentity(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "3")
		if r.Method == "HEAD" {
			return
		}
		if strings.Contains(r.Header.Get("Authorization"), "blog-key:") {
			io.WriteString(w, "new")
		} else {
			io.WriteString(w, "old")
		}
	}))
	defer storage.Close()
	cfg, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.PictureProxy.Enabled = true
	cfg.PictureProxy.Endpoint = storage.URL
	cfg.PictureProxy.Bucket = "test-bucket"
	cfg.PictureProxy.AccessKeyID = "old-key"
	cfg.PictureProxy.AccessKeySecret = "old-secret"
	cfg.BlogMedia = config.BlogMedia{Endpoint: storage.URL, Bucket: "test-bucket", AccessKeyID: "blog-key", AccessKeySecret: "blog-secret"}
	h, err := newPictureHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{{"/blog/status/a.png", "new"}, {"/pic/a.png", "old"}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
		if rec.Code != 200 || rec.Body.String() != tc.want {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
}
