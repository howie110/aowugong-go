package httpserver

import (
	"encoding/xml"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
)

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	Generator     string    `xml:"generator"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	Author      string  `xml:"author,omitempty"`
	PubDate     string  `xml:"pubDate"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// registerArticleFeedRoutes 注册供服务器本机 Miniflux 读取的微信公众号 RSS。
// 输入：router 是 API 路由器，service 提供持久文章查询。
// 输出：无。
// 副作用：修改路由注册表。
func registerArticleFeedRoutes(router chi.Router, service *articleanalysis.Service) {
	// 1. RSS 不使用工作台 JWT，仅允许回环地址访问。
	router.Get("/feeds/weread.xml", articleFeedHandler(service))
}

// articleFeedHandler 输出最新微信公众号文章的 RSS 2.0 文档。
// 输入：service 提供文章；请求必须来自服务器本机。
// 输出：成功返回 RSS XML，非本机返回 403，查询失败返回 500。
// 副作用：只读 PostgreSQL 并写入 HTTP 响应。
func articleFeedHandler(service *articleanalysis.Service) http.HandlerFunc {
	// 1. 固定依赖并为每次请求执行来源限制、查询和 XML 编码。
	return func(w http.ResponseWriter, request *http.Request) {
		// 1.1. 拒绝公网访问，避免公众号正文通过工作台端口公开暴露。
		if !isLoopbackRequest(request) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		articles, err := service.WeReadFeedArticles(request.Context())
		if err != nil {
			http.Error(w, "RSS unavailable", http.StatusInternalServerError)
			return
		}

		// 1.2. 使用最新文章和数量生成弱 ETag，未变化时让 Miniflux 跳过正文传输。
		latestID := int64(0)
		if len(articles) > 0 {
			latestID = articles[0].ID
		}
		etag := fmt.Sprintf(`W/"weread-%d-%d"`, latestID, len(articles))
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, max-age=300")
		if request.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		// 1.3. 构造 RSS 并一次写入，避免部分 XML 响应。
		document := buildWeReadRSS(request, articles, time.Now())
		content, err := xml.Marshal(document)
		if err != nil {
			http.Error(w, "RSS unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(append([]byte(xml.Header), content...))
	}
}

// buildWeReadRSS 把数据库文章转换为 RSS 2.0 文档。
// 输入：request 提供本机服务地址，articles 是倒序文章，now 是构建时间。
// 输出：返回可由 encoding/xml 编码的 RSS 文档。
// 副作用：无。
func buildWeReadRSS(request *http.Request, articles []articleanalysis.FeedArticle, now time.Time) rssDocument {
	// 1. 使用请求地址生成频道链接，并逐篇转换稳定 GUID、时间和 HTML 正文。
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	items := make([]rssItem, 0, len(articles))
	for _, article := range articles {
		publishedAt := parseRSSArticleTime(article.PublishedAt)
		items = append(items, rssItem{
			Title: article.Title, Link: article.Link,
			GUID:   rssGUID{IsPermaLink: false, Value: fmt.Sprintf("aowugong:weread:%d:%s", article.ID, article.ArticleKey)},
			Author: article.Author, PubDate: publishedAt.Format(time.RFC1123Z),
			Description: rssArticleHTML(article.Content, article.Summary),
		})
	}
	return rssDocument{Version: "2.0", Channel: rssChannel{
		Title: "Aowugong 微信公众号", Link: scheme + "://" + request.Host + "/article-analysis",
		Description: "Aowugong 从微信读书书架获取的微信公众号文章。",
		Language:    "zh-CN", Generator: "aowugong-go", LastBuildDate: now.Format(time.RFC1123Z), Items: items,
	}}
}

// parseRSSArticleTime 把数据库时间转换为带上海时区的 RSS 时间。
// 输入：value 是 RFC3339、数据库日期时间或日期文本。
// 输出：返回解析时间；无法解析时返回 Unix 纪元时间。
// 副作用：无。
func parseRSSArticleTime(value string) time.Time {
	// 1. 按数据库实际格式从精确到宽松依次解析。
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed
		}
	}
	return time.Unix(0, 0).UTC()
}

// rssArticleHTML 把纯文本正文转换为 Miniflux 可展示的安全 HTML。
// 输入：content 是正文，summary 是正文缺失时的摘要。
// 输出：返回转义后包在段落中的 HTML。
// 副作用：无。
func rssArticleHTML(content, summary string) string {
	// 1. 优先使用正文，统一换行后转义，避免文章文本注入 RSS HTML。
	text := strings.TrimSpace(content)
	if text == "" {
		text = strings.TrimSpace(summary)
	}
	escaped := html.EscapeString(text)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	paragraphs := strings.Split(escaped, "\n\n")
	for index, paragraph := range paragraphs {
		paragraphs[index] = "<p>" + strings.ReplaceAll(strings.TrimSpace(paragraph), "\n", "<br>") + "</p>"
	}
	return strings.Join(paragraphs, "")
}

// isLoopbackRequest 判断请求是否直接来自服务器本机。
// 输入：request.RemoteAddr 是连接对端地址。
// 输出：IPv4 或 IPv6 回环地址返回 true。
// 副作用：无。
func isLoopbackRequest(request *http.Request) bool {
	// 1. 拆分端口并使用标准 IP 语义判断，拒绝伪造 Host 或转发头。
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(request.RemoteAddr), "[]")
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
