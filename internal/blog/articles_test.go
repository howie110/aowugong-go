package blog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const articleFixture = "---\ntitle: 测试文章\ndate: 2026-09-30\ntags: [博客, life]\nimage: https://pic.aowugong.top/pic/test.jpg\n---\n\n## 第一节\n第一行\n第二行\n\n[网站](https://example.com)\n\n```go\nfmt.Println(1)\n```\n"

func writeArticle(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestArticleStoreReadsFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "hello.md", articleFixture)
	s := NewArticleStore(dir)
	a, err := s.Get("hello")
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "测试文章" || a.Date != "2026-09-30" || len(a.Tags) != 1 || a.Tags[0] != "life" {
		t.Fatalf("metadata: %+v", a)
	}
	if !strings.Contains(a.HTML, "<br") || !strings.Contains(a.HTML, "language-go") || len(a.TOC) != 1 || a.TOC[0].Text != "第一节" {
		t.Fatalf("body/toc: %+v", a)
	}
	if !strings.Contains(a.HTML, `id="`+a.TOC[0].ID+`"`) {
		t.Fatal("TOC anchor missing")
	}
	posts, err := s.List()
	if err != nil || len(posts) != 1 {
		t.Fatalf("list %v %v", posts, err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestArticleStoreRejectsTraversalAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeArticle(t, outside, "private.md", articleFixture)
	s := NewArticleStore(dir)
	for _, slug := range []string{"../private", "/private", "a/../../private", `a\b`} {
		if _, err := s.Get(slug); err == nil {
			t.Fatalf("accepted %q", slug)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "private.md"), filepath.Join(dir, "linked.md")); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err == nil {
		t.Fatal("accepted symlink")
	}
}
func TestArticleStoreReportsMalformedFile(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "bad.md", "---\ntitle: Bad\ndate: nope\n---\ntext")
	if err := NewArticleStore(dir).Validate(); err == nil || !strings.Contains(err.Error(), "bad.md") {
		t.Fatalf("error=%v", err)
	}
}
func TestMarkdownRejectsExecutableHTML(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "safe.md", articleFixture+"\n<script>alert(1)</script>\n<img src=x onerror=alert(1)>\n[x](javascript:alert(1))")
	a, err := NewArticleStore(dir).Get("safe")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "onerror=", "javascript:"} {
		if strings.Contains(a.HTML, bad) {
			t.Fatalf("unsafe html %s", a.HTML)
		}
	}
}
func TestArticleStoreRejectsEscapingAttachments(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "bad.md", articleFixture+"\n![private](../secret.png)")
	if err := NewArticleStore(dir).Validate(); err == nil {
		t.Fatal("accepted outside image")
	}
}
func TestArticleStoreSkipsLegacyStatus(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "hello.md", articleFixture)
	writeArticle(t, dir, "blog-2000-01-05.md", "legacy")
	articles, err := NewArticleStore(dir).List()
	if err != nil || len(articles) != 1 {
		t.Fatalf("%v %v", articles, err)
	}
}

func TestArticleYAMLTimestampPreservesCalendarDate(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "timestamp.md", strings.Replace(articleFixture, "2026-09-30", "2024-03-20 00:00:00", 1))
	a, err := NewArticleStore(dir).Get("timestamp")
	if err != nil {
		t.Fatal(err)
	}
	if a.Date != "2024-03-20" {
		t.Fatalf("date %s", a.Date)
	}
}
