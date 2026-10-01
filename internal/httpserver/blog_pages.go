package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/blog"
)

const blogPublicURL = "https://aowugong.top"

var titleElement = regexp.MustCompile(`(?s)<title>.*?</title>`)

type blogPages struct {
	articles  *blog.ArticleRepository
	staticDir string
}

func (p blogPages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, 405, "method_not_allowed", "仅支持读取")
		return
	}
	requestPath := strings.TrimRight(r.URL.Path, "/")
	if requestPath == "/blog/posts/blog-2000-01-05" {
		http.Redirect(w, r, "/blog/status", http.StatusMovedPermanently)
		return
	}
	if strings.HasPrefix(requestPath, "/blog/assets/") {
		file, err := p.articles.Asset(r.Context(), strings.TrimPrefix(requestPath, "/blog/assets/"))
		if err != nil {
			blogError(w, err)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(file.Content)))
		http.ServeContent(w, r, filepath.Base(file.Path), time.Time{}, bytes.NewReader(file.Content))
		return
	}
	if requestPath == "/blog/rss.xml" || requestPath == "/blog/sitemap.xml" {
		p.feed(w, r, requestPath)
		return
	}
	title, description := "嗷呜公 · 博客", "熬一些鸡汤，记录生活与思考。"
	switch {
	case requestPath == "/blog":
	case requestPath == "/blog/status":
		title = "状态 · 嗷呜公"
	case requestPath == "/blog/tags" || strings.HasPrefix(requestPath, "/blog/tags/"):
		title = "标签 · 嗷呜公"
	case strings.HasPrefix(requestPath, "/blog/posts/"):
		article, err := p.articles.Get(r.Context(), strings.TrimPrefix(requestPath, "/blog/posts/"))
		if err != nil {
			blogError(w, err)
			return
		}
		title = article.Title + " · 嗷呜公"
		description = article.Description
	default:
		blogError(w, blog.ErrNotFound)
		return
	}
	document, err := os.ReadFile(filepath.Join(p.staticDir, "index.html"))
	if err != nil {
		blogError(w, err)
		return
	}
	rendered := titleElement.ReplaceAllStringFunc(string(document), func(string) string { return "<title>" + html.EscapeString(title) + "</title>" })
	canonical := blogPublicURL + (&url.URL{Path: requestPath}).EscapedPath()
	meta := fmt.Sprintf(`<meta name="description" content="%s"><link rel="canonical" href="%s"><link rel="alternate" type="application/rss+xml" title="嗷呜公" href="/blog/rss.xml"><meta property="og:title" content="%s"><meta property="og:description" content="%s"><meta property="og:url" content="%s">`, html.EscapeString(description), html.EscapeString(canonical), html.EscapeString(title), html.EscapeString(description), html.EscapeString(canonical))
	rendered = strings.Replace(rendered, "</head>", meta+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != "HEAD" {
		_, _ = w.Write([]byte(rendered))
	}
}

type blogRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Date        string `xml:"pubDate"`
}
type blogRSSChannel struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description string        `xml:"description"`
	Items       []blogRSSItem `xml:"item"`
}
type blogRSSDocument struct {
	XMLName xml.Name       `xml:"rss"`
	Version string         `xml:"version,attr"`
	Channel blogRSSChannel `xml:"channel"`
}
type sitemapURL struct {
	Location string `xml:"loc"`
	Modified string `xml:"lastmod,omitempty"`
}
type sitemapDocument struct {
	XMLName   xml.Name     `xml:"urlset"`
	Namespace string       `xml:"xmlns,attr"`
	URLs      []sitemapURL `xml:"url"`
}

func (p blogPages) feed(w http.ResponseWriter, r *http.Request, requestPath string) {
	articles, err := p.articles.List(r.Context())
	if err != nil {
		blogError(w, err)
		return
	}
	rss := blogRSSDocument{Version: "2.0", Channel: blogRSSChannel{Title: "嗷呜公", Link: blogPublicURL + "/blog", Description: "熬一些鸡汤"}}
	sitemap := sitemapDocument{Namespace: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: []sitemapURL{{Location: blogPublicURL + "/blog"}, {Location: blogPublicURL + "/blog/status"}, {Location: blogPublicURL + "/blog/tags"}}}
	for _, a := range articles {
		link := blogPublicURL + "/blog/posts/" + (&url.URL{Path: a.Slug}).EscapedPath()
		date, _ := time.Parse("2006-01-02", a.Date)
		rss.Channel.Items = append(rss.Channel.Items, blogRSSItem{Title: a.Title, Link: link, GUID: link, Description: a.HTML, Date: date.Format(time.RFC1123Z)})
		sitemap.URLs = append(sitemap.URLs, sitemapURL{Location: link, Modified: a.Date})
	}
	var value any = rss
	if requestPath == "/blog/sitemap.xml" {
		value = sitemap
	}
	data, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		blogError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != "HEAD" {
		_, _ = w.Write(append([]byte(xml.Header), data...))
	}
}
