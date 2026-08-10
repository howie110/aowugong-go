package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMinifluxClientFetchesCategoryEntries 验证 Miniflux 客户端只读取指定分类并规范化文章。
// 输入：提供分类和文章 API 的本地 HTTP 服务。
// 输出：请求携带 API Token 和分类 ID，返回现有文章服务可直接入库的字段。
// 副作用：启动测试 HTTP 服务并发起本机请求。
func TestMinifluxClientFetchesCategoryEntries(t *testing.T) {
	// 1. 模拟完整的 Miniflux 分类和文章响应，并核对请求契约。
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Auth-Token") != "test-token" {
			t.Errorf("X-Auth-Token = %q", request.Header.Get("X-Auth-Token"))
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/categories":
			_, _ = response.Write([]byte(`[
				{"id":11,"title":"普通订阅","user_id":1,"hide_globally":false},
				{"id":22,"title":"投资文章","user_id":1,"hide_globally":false}
			]`))
		case "/v1/entries":
			query := request.URL.Query()
			if query.Get("category_id") != "22" || query.Get("limit") != "30" ||
				query.Get("order") != "published_at" || query.Get("direction") != "desc" {
				t.Errorf("entries query = %q", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{
				"total":1,
				"entries":[{
					"id":888,
					"user_id":1,
					"feed_id":42,
					"title":"测试投资文章",
					"url":"https://example.com/article-888",
					"comments_url":"",
					"author":"完整作者名",
					"content":"<p>第一段</p><p>第二段</p>",
					"hash":"29f99e4074cdacca1766f47697d03c66070ef6a14770a1fd5a867483c207a1bb",
					"published_at":"2026-08-07T00:15:19+08:00",
					"created_at":"2026-08-07T08:16:19+08:00",
					"status":"unread",
					"share_code":"",
					"starred":false,
					"reading_time":1,
					"enclosures":null,
					"feed":{
						"id":42,
						"user_id":1,
						"title":"测试公众号",
						"site_url":"https://example.com",
						"feed_url":"https://example.com/feed.xml",
						"checked_at":"2026-08-07T08:20:00+08:00",
						"etag_header":"",
						"last_modified_header":"",
						"parsing_error_message":"",
						"parsing_error_count":0,
						"scraper_rules":"",
						"rewrite_rules":"",
						"crawler":false,
						"blocklist_rules":"",
						"keeplist_rules":"",
						"user_agent":"",
						"username":"",
						"password":"",
						"disabled":false,
						"ignore_http_cache":false,
						"fetch_via_proxy":false,
						"category":{"id":22,"title":"投资文章","user_id":1,"hide_globally":false}
					}
				}]
			}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	// 2. 读取指定分类并核对现有文章模型需要的稳定字段。
	client := NewMinifluxClient(server.URL, "test-token", "投资文章", server.Client())
	items, err := client.Fetch(context.Background(), 7, server.URL, 30)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.ExternalID != "888" || item.Title != "测试投资文章" || item.Author != "完整作者名" {
		t.Errorf("identity = %#v", item)
	}
	if item.Link != "https://example.com/article-888" || item.PublishedAt != "2026-08-07 00:15:19" {
		t.Errorf("link/date = %q/%q", item.Link, item.PublishedAt)
	}
	if item.Summary != "第一段 第二段" || item.Content != "第一段 第二段" || len(item.ArticleKey) != 64 {
		t.Errorf("content/key = %q/%q/%q", item.Summary, item.Content, item.ArticleKey)
	}
}

// TestMinifluxClientClampsLimitToAPIMaximum 验证文章请求不会超过 Miniflux 允许的单页上限。
// 输入：调用方请求 2000 篇文章，测试服务模拟分类和文章 API。
// 输出：实际文章请求使用 limit=1000，并正常返回空文章列表。
// 副作用：启动本地测试 HTTP 服务并发起两次请求。
func TestMinifluxClientClampsLimitToAPIMaximum(t *testing.T) {
	// 1. 模拟 Miniflux API，并核对文章列表请求的最终上限。
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/categories":
			_, _ = response.Write([]byte(`[{"id":22,"title":"投资文章"}]`))
		case "/v1/entries":
			if got := request.URL.Query().Get("limit"); got != "1000" {
				t.Errorf("limit = %q, want 1000", got)
			}
			_, _ = response.Write([]byte(`{"total":0,"entries":[]}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	// 2. 请求超过服务端边界的数量，确认客户端完成收敛且不报错。
	client := NewMinifluxClient(server.URL, "test-token", "投资文章", server.Client())
	items, err := client.Fetch(context.Background(), 7, server.URL, 2000)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want empty", items)
	}
}
