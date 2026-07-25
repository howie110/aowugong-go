package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRSSClientPollsAndParsesFeed 验证 WeChatRSS 轮询和 RSS 文章规范化。
// 输入：提供轮询 JSON 和 RSS XML 的本地 HTTP 服务。
// 输出：轮询成功，并返回标题、作者、正文和稳定文章键。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestRSSClientPollsAndParsesFeed(t *testing.T) {
	// 1. 创建同时提供轮询和聚合 RSS 的测试服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/rss/poll" {
			if request.Method != http.MethodPost {
				t.Errorf("poll method = %s, want POST", request.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>article-1</guid><title>测试文章</title><link>https://example.com/1</link><author>完整作者名</author><pubDate>Wed, 15 Jul 2026 08:00:00 +0800</pubDate><description><![CDATA[<p>摘要 正文</p>]]></description></item></channel></rss>`))
	}))
	defer server.Close()
	client := NewRSSClient(server.Client())

	// 2. 触发上游刷新并读取一篇文章。
	if err := client.Poll(context.Background(), server.URL+"/rss/all.xml"); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	items, err := client.Fetch(context.Background(), 7, server.URL+"/rss/all.xml", 30)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	// 3. 核对规范化结果和稳定键。
	if len(items) != 1 || items[0].Title != "测试文章" || items[0].Author != "完整作者名" || items[0].Content != "摘要 正文" {
		t.Fatalf("items = %#v", items)
	}
	if len(items[0].ArticleKey) != 64 || items[0].PublishedAt == "" {
		t.Errorf("item key/date = %q/%q", items[0].ArticleKey, items[0].PublishedAt)
	}
}

// TestParseFeedTimeReturnsSQLiteCompatibleUTC 验证 feed 时间转换为 SQLite DATETIME 可写入的 UTC 文本。
// 输入：提供等价于线上异常值的东八区 RSS 时间。
// 输出：返回 YYYY-MM-DD HH:MM:SS，不包含 RFC3339 的 T 和 Z。
// 副作用：无。
func TestParseFeedTimeReturnsSQLiteCompatibleUTC(t *testing.T) {
	// 1. 解析线上文章对应的东八区发布时间。
	got := parseFeedTime("Thu, 16 Jul 2026 14:44:07 +0800")

	// 2. 核对结果保留旧 FastAPI 的 UTC 无时区入库语义。
	want := "2026-07-16 06:44:07"
	if got != want {
		t.Fatalf("parseFeedTime() = %q, want %q", got, want)
	}
}
