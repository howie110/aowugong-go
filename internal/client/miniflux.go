package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MinifluxClient 从 Miniflux API 读取指定分类的文章。
type MinifluxClient struct {
	baseURL      string
	apiToken     string
	categoryName string
	httpClient   *http.Client
}

type minifluxCategory struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type minifluxEntriesResponse struct {
	Total   int             `json:"total"`
	Entries []minifluxEntry `json:"entries"`
}

type minifluxEntry struct {
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	URL         string       `json:"url"`
	Author      string       `json:"author"`
	Content     string       `json:"content"`
	Hash        string       `json:"hash"`
	PublishedAt string       `json:"published_at"`
	CreatedAt   string       `json:"created_at"`
	Status      string       `json:"status"`
	Feed        minifluxFeed `json:"feed"`
}

type minifluxFeed struct {
	ID       int64            `json:"id"`
	Title    string           `json:"title"`
	SiteURL  string           `json:"site_url"`
	FeedURL  string           `json:"feed_url"`
	Category minifluxCategory `json:"category"`
}

// NewMinifluxClient 创建按分类读取文章的 Miniflux 客户端。
// 输入：baseURL 是 Miniflux 根地址，apiToken 是应用密钥，categoryName 是目标分类，httpClient 提供超时控制。
// 输出：返回可并发复用的客户端。
// 副作用：无，不立即访问外部服务。
func NewMinifluxClient(baseURL, apiToken, categoryName string, httpClient *http.Client) *MinifluxClient {
	// 1. 补齐默认 HTTP 超时并清理配置文本。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &MinifluxClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), apiToken: strings.TrimSpace(apiToken),
		categoryName: strings.TrimSpace(categoryName), httpClient: httpClient,
	}
}

// Fetch 读取 Miniflux 指定分类的最新文章并转换为统一文章结构。
// 输入：ctx 控制请求，sourceID 参与稳定去重，feedURL 仅作根地址回退，limit 是最大文章数。
// 输出：返回按发布时间倒序的文章；配置、鉴权或响应无效时返回错误。
// 副作用：调用 Miniflux 分类和文章 API。
func (c *MinifluxClient) Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]ArticleItem, error) {
	// 1. 校验连接配置和数量边界。
	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(feedURL), "/")
	}
	if baseURL == "" || c.apiToken == "" {
		return nil, fmt.Errorf("Miniflux 地址或 API Token 未配置")
	}
	if c.categoryName == "" {
		return nil, fmt.Errorf("Miniflux 投资文章分类未配置")
	}
	if limit < 1 {
		limit = 30
	}
	if limit > 1000 {
		limit = 1000
	}

	// 2. 解析分类名称，确保不会把其他订阅误送入投资分析。
	categoryID, err := c.findCategoryID(ctx, baseURL)
	if err != nil {
		return nil, err
	}

	// 3. 请求指定分类的最新文章。
	endpoint, err := url.Parse(baseURL + "/v1/entries")
	if err != nil {
		return nil, fmt.Errorf("解析 Miniflux 文章地址: %w", err)
	}
	query := endpoint.Query()
	query.Set("category_id", strconv.FormatInt(categoryID, 10))
	query.Set("limit", strconv.Itoa(limit))
	query.Set("order", "published_at")
	query.Set("direction", "desc")
	endpoint.RawQuery = query.Encode()
	var payload minifluxEntriesResponse
	if err := c.getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, fmt.Errorf("读取 Miniflux 投资文章: %w", err)
	}

	// 4. 转换为文章服务现有模型。
	items := make([]ArticleItem, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		items = append(items, normalizeMinifluxEntry(sourceID, entry))
	}
	return items, nil
}

// findCategoryID 按名称查找 Miniflux 分类主键。
// 输入：ctx 控制请求，baseURL 是已清理的根地址。
// 输出：返回匹配分类 ID；不存在时返回可读错误。
// 副作用：调用 Miniflux 分类 API。
func (c *MinifluxClient) findCategoryID(ctx context.Context, baseURL string) (int64, error) {
	// 1. 读取分类并精确匹配配置名称。
	var categories []minifluxCategory
	if err := c.getJSON(ctx, baseURL+"/v1/categories", &categories); err != nil {
		return 0, fmt.Errorf("读取 Miniflux 分类: %w", err)
	}
	for _, category := range categories {
		if strings.TrimSpace(category.Title) == c.categoryName {
			return category.ID, nil
		}
	}
	return 0, fmt.Errorf("Miniflux 中不存在分类 %q", c.categoryName)
}

// getJSON 执行带应用密钥的 Miniflux GET 请求并解码 JSON。
// 输入：ctx 控制请求，endpoint 是完整地址，target 接收 JSON。
// 输出：成功返回 nil；HTTP 或 JSON 异常时返回错误。
// 副作用：发起外部 HTTP GET。
func (c *MinifluxClient) getJSON(ctx context.Context, endpoint string, target any) error {
	// 1. 创建请求并写入 Miniflux 应用认证头。
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建 Miniflux 请求: %w", err)
	}
	request.Header.Set("X-Auth-Token", c.apiToken)
	request.Header.Set("User-Agent", "aowugong-go/1.0")

	// 2. 限制响应大小并统一处理非成功状态。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("请求 Miniflux: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 50<<20))
	if err != nil {
		return fmt.Errorf("读取 Miniflux 响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Miniflux HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	// 3. 解码调用方指定的响应结构。
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 Miniflux JSON: %w", err)
	}
	return nil
}

// normalizeMinifluxEntry 把 Miniflux 条目转换为现有文章模型。
// 输入：sourceID 参与去重，entry 是 API 条目。
// 输出：返回已清理正文、UTC 时间和稳定文章键。
// 副作用：无。
func normalizeMinifluxEntry(sourceID int64, entry minifluxEntry) ArticleItem {
	// 1. 清理正文并构造稳定的外部主键。
	externalID := strconv.FormatInt(entry.ID, 10)
	content := truncateClientRunes(htmlToText(entry.Content), 20000)
	summary := truncateClientRunes(content, 2000)
	publishedAt := parseFeedTime(firstNonEmpty(entry.PublishedAt, entry.CreatedAt))

	// 2. 保留来源审计字段并返回统一结构。
	return ArticleItem{
		ArticleKey: buildArticleKey(sourceID, externalID), ExternalID: externalID,
		Title: normalizeFeedText(entry.Title), Link: normalizeFeedText(entry.URL),
		Author: normalizeFeedText(entry.Author), PublishedAt: publishedAt,
		Summary: summary, Content: content,
		RawEntry: map[string]any{
			"id": entry.ID, "hash": entry.Hash, "status": entry.Status,
			"feed_id": entry.Feed.ID, "feed_title": entry.Feed.Title,
			"category": entry.Feed.Category.Title, "published_at": publishedAt,
		},
	}
}
