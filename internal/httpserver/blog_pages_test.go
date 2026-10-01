package httpserver

import (
	"context"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/blog"
)

func TestBlogPagesMetadataFeedsAnd404(t *testing.T) {
	dir := t.TempDir()
	static := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.md"), []byte("---\ntitle: 'Hello <world>'\ndate: 2026-09-30\ntags: [life]\n---\n# Heading\nContent\n![正文首图](https://example.com/first.jpg)\n![第二张图](https://example.com/second.jpg)"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(static, "index.html"), []byte(`<html><head><title>Workbench</title></head><body><div id="root"></div></body></html>`), 0600); err != nil {
		t.Fatal(err)
	}
	repo := blog.NewArticleRepository(testdatabase.Open(t))
	if err := repo.PublishDirectory(context.Background(), dir, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Dependencies{StaticDir: static, BlogArticles: repo})
	for _, tc := range []struct {
		path     string
		want     int
		contains string
	}{{"/api/v1/blog/posts", 200, `"thumbnail":"https://example.com/first.jpg"`}, {"/blog", 200, "嗷呜公"}, {"/blog/posts/hello", 200, "Hello &lt;world&gt;"}, {"/blog/posts/missing", 404, "不存在"}, {"/blog/unknown", 404, "不存在"}, {"/blog/rss.xml", 200, "https://aowugong.top/blog/posts/hello"}, {"/blog/sitemap.xml", 200, "https://aowugong.top/blog/posts/hello"}, {"/blog/posts/blog-2000-01-05", 301, "/blog/status"}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
		if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.contains) {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
		if strings.HasPrefix(tc.path, "/blog/posts/hello") && !strings.Contains(rec.Body.String(), `rel="canonical" href="https://aowugong.top/blog/posts/hello"`) {
			t.Fatal("missing canonical")
		}
	}
}
