package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestArticleFeedOnlyServesLoopbackMiniflux 验证公众号 RSS 仅供本机读取且包含稳定文章内容。
// 输入：隔离数据库内的一篇微信读书文章，以及本机和公网两种请求地址。
// 输出：本机返回 RSS 2.0，公网返回 403。
// 副作用：写入隔离 SQLite 并执行两次内存 HTTP 请求。
func TestArticleFeedOnlyServesLoopbackMiniflux(t *testing.T) {
	// 1. 创建两个微信读书公众号和一篇含特殊字符及折叠段落的文章。
	db := testdatabase.Open(t)
	repository := articleanalysis.NewRepository(db)
	if err := repository.SetWeReadSourceActive(context.Background(), true); err != nil {
		t.Fatalf("SetWeReadSourceActive() error = %v", err)
	}
	var sourceID int64
	if err := db.QueryRow("SELECT id FROM investment_article_source WHERE source_type='weread'").Scan(&sourceID); err != nil {
		t.Fatalf("query source ID: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO weread_article_account(account_id, title) VALUES (?, ?), (?, ?)`,
		"MP_WXS_1001", "猫笔刀", "MP_WXS_1002", "野生国师"); err != nil {
		t.Fatalf("insert WeRead accounts: %v", err)
	}
	if _, _, err := repository.UpsertArticle(context.Background(), sourceID, articleanalysis.FeedEntry{
		ArticleKey: "weread:test-1", Title: "测试 & 文章", Link: "https://mp.weixin.qq.com/s/test",
		Author: "猫笔刀", PublishedAt: "2026-08-15 08:00:00", Summary: "测试摘要", Content: "第一段 <正文>。 第二段！ 第三段",
	}); err != nil {
		t.Fatalf("UpsertArticle() error = %v", err)
	}
	if _, _, err := repository.UpsertArticle(context.Background(), sourceID, articleanalysis.FeedEntry{
		ArticleKey: "weread:test-2", Title: "其他公众号文章", Link: "https://mp.weixin.qq.com/s/other",
		Author: "野生国师", PublishedAt: "2026-08-15 09:00:00", Content: "不应出现在猫笔刀订阅源",
	}); err != nil {
		t.Fatalf("UpsertArticle() other account error = %v", err)
	}
	service := articleanalysis.NewService(repository, articleanalysis.ServiceOptions{})
	handler := NewRouter(Dependencies{ArticleAnalysis: service})

	// 2. 本机请求应得到 Miniflux 可解析的 RSS、作者、稳定 GUID 和转义正文。
	localRequest := httptest.NewRequest(http.MethodGet, "/feeds/weread/MP_WXS_1001.xml", nil)
	localRequest.RemoteAddr = "127.0.0.1:34567"
	localRecorder := httptest.NewRecorder()
	handler.ServeHTTP(localRecorder, localRequest)
	if localRecorder.Code != http.StatusOK {
		t.Fatalf("local status = %d, body = %s", localRecorder.Code, localRecorder.Body.String())
	}
	body := localRecorder.Body.String()
	for _, fragment := range []string{
		`<rss version="2.0">`, `<channel><title>猫笔刀</title>`, `<title>测试 &amp; 文章</title>`, `<author>猫笔刀</author>`,
		`<guid isPermaLink="false">aowugong:weread:`, `&lt;p&gt;第一段 &amp;lt;正文&amp;gt;。&lt;/p&gt;&lt;p&gt;第二段！&lt;/p&gt;&lt;p&gt;第三段&lt;/p&gt;`,
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("RSS is missing %q: %s", fragment, body)
		}
	}
	if contentType := localRecorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/rss+xml") {
		t.Errorf("Content-Type = %q", contentType)
	}
	if strings.Contains(body, "其他公众号文章") {
		t.Errorf("RSS contains another account article: %s", body)
	}

	// 3. 未知公众号返回 404，非回环连接即使使用合法 URL 也不能读取文章正文。
	missingRequest := httptest.NewRequest(http.MethodGet, "/feeds/weread/MP_WXS_9999.xml", nil)
	missingRequest.RemoteAddr = "127.0.0.1:34567"
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", missingRecorder.Code, http.StatusNotFound)
	}

	externalRequest := httptest.NewRequest(http.MethodGet, "/feeds/weread/MP_WXS_1001.xml", nil)
	externalRequest.RemoteAddr = "203.0.113.8:45678"
	externalRecorder := httptest.NewRecorder()
	handler.ServeHTTP(externalRecorder, externalRequest)
	if externalRecorder.Code != http.StatusForbidden {
		t.Fatalf("external status = %d, want %d", externalRecorder.Code, http.StatusForbidden)
	}
}
