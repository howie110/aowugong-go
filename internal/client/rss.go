package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	htmlTagPattern  = regexp.MustCompile(`<[^>]+>`)
	spaceRunPattern = regexp.MustCompile(`\s+`)
)

// RSSItem 描述 RSS/Atom 客户端规范化的一篇文章。
type RSSItem struct {
	ArticleKey  string
	ExternalID  string
	Title       string
	Link        string
	Author      string
	PublishedAt string
	Summary     string
	Content     string
	RawEntry    map[string]any
}

// RSSClient 抓取 RSS/Atom 并触发 WeChatRSS 刷新。
type RSSClient struct {
	httpClient *http.Client
}

type feedDocument struct {
	Channel feedChannel `xml:"channel"`
	Entries []feedNode  `xml:"entry"`
}

type feedChannel struct {
	Items []feedNode `xml:"item"`
}

type feedNode struct {
	GUID        string     `xml:"guid"`
	ID          string     `xml:"id"`
	Title       string     `xml:"title"`
	Links       []feedLink `xml:"link"`
	Author      feedAuthor `xml:"author"`
	Creator     string     `xml:"creator"`
	PubDate     string     `xml:"pubDate"`
	Published   string     `xml:"published"`
	Updated     string     `xml:"updated"`
	Description string     `xml:"description"`
	Summary     string     `xml:"summary"`
	Encoded     string     `xml:"encoded"`
	Content     string     `xml:"content"`
}

type feedLink struct {
	Href string `xml:"href,attr"`
	Text string `xml:",chardata"`
}

type feedAuthor struct {
	Name string `xml:"name"`
	Text string `xml:",chardata"`
}

// NewRSSClient 创建 RSS 和 WeChatRSS 客户端。
// 输入：httpClient 提供超时和连接复用；nil 时使用 30 秒默认客户端。
// 输出：返回可并发复用客户端。
// 副作用：无。
func NewRSSClient(httpClient *http.Client) *RSSClient {
	// 1. 应用默认超时并保存依赖。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &RSSClient{httpClient: httpClient}
}

// Poll 触发 WeChatRSS 手动轮询订阅文章。
// 输入：ctx 控制请求，feedURL 是同一服务上的任意 RSS 地址。
// 输出：成功返回 nil，HTTP、JSON 或业务失败时返回错误。
// 副作用：调用 WeChatRSS 的 /api/rss/poll。
func (c *RSSClient) Poll(ctx context.Context, feedURL string) error {
	// 1. 从 feed 地址构造同源轮询接口。
	parsed, err := url.Parse(feedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("WeChatRSS 地址无效: %s", feedURL)
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "/api/rss/poll", "", ""
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("创建 WeChatRSS 轮询请求: %w", err)
	}
	request.Header.Set("User-Agent", "aowugong-go/1.0")

	// 2. 请求并解析成功标记。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("请求 WeChatRSS 轮询接口: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取 WeChatRSS 轮询响应: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("WeChatRSS 轮询接口异常: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("WeChatRSS 轮询接口返回非 JSON: %w", err)
	}
	if success, _ := payload["success"].(bool); !success {
		return fmt.Errorf("WeChatRSS 轮询未成功: %s", responseMessage(payload))
	}
	return nil
}

// Fetch 抓取 RSS 或 Atom 并规范化文章。
// 输入：ctx 控制请求，sourceID 参与去重键，feedURL 是地址，limit 是最多 100 篇。
// 输出：返回按 feed 顺序的文章；请求或 XML 无效时返回错误。
// 副作用：发起 RSS HTTP GET。
func (c *RSSClient) Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]RSSItem, error) {
	// 1. 校验参数并发起带上下文 GET。
	if strings.TrimSpace(feedURL) == "" {
		return nil, fmt.Errorf("RSS 地址为空")
	}
	if limit < 1 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 RSS 请求: %w", err)
	}
	request.Header.Set("User-Agent", "aowugong-go/1.0")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("抓取 RSS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("抓取 RSS: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	// 2. 限制 XML 大小并解码 RSS/Atom 节点。
	var document feedDocument
	decoder := xml.NewDecoder(io.LimitReader(response.Body, 20<<20))
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("解析 RSS XML: %w", err)
	}
	nodes := document.Channel.Items
	if len(nodes) == 0 {
		nodes = document.Entries
	}
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	// 3. 逐节点转换为统一文章结构。
	results := make([]RSSItem, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, normalizeFeedNode(sourceID, node))
	}
	return results, nil
}

// normalizeFeedNode 把 RSS/Atom 节点转换为统一文章。
// 输入：sourceID 参与去重，node 是 XML 节点。
// 输出：返回正文和稳定键均已处理的文章。
// 副作用：无。
func normalizeFeedNode(sourceID int64, node feedNode) RSSItem {
	// 1. 读取 RSS/Atom 基础字段和时间。
	title := normalizeFeedText(node.Title)
	if title == "" {
		title = "未命名文章"
	}
	link := ""
	for _, item := range node.Links {
		if strings.TrimSpace(item.Href) != "" {
			link = normalizeFeedText(item.Href)
			break
		}
		if link == "" && strings.TrimSpace(item.Text) != "" {
			link = normalizeFeedText(item.Text)
		}
	}
	externalID := normalizeFeedText(firstNonEmpty(node.GUID, node.ID, link, title))
	author := normalizeFeedText(firstNonEmpty(node.Author.Name, node.Author.Text, node.Creator))
	publishedAt := parseFeedTime(firstNonEmpty(node.PubDate, node.Published, node.Updated))
	rawSummary := firstNonEmpty(node.Description, node.Summary)
	rawContent := firstNonEmpty(node.Encoded, node.Content, rawSummary)
	summary := truncateClientRunes(htmlToText(rawSummary), 2000)
	content := truncateClientRunes(htmlToText(rawContent), 20000)

	// 2. 构造稳定 SHA-256 键和审计字段。
	seed := strconv.FormatInt(sourceID, 10) + "|" + firstNonEmpty(externalID, link, title)
	hash := sha256.Sum256([]byte(seed))
	return RSSItem{
		ArticleKey: hex.EncodeToString(hash[:]), ExternalID: externalID, Title: title, Link: link,
		Author: author, PublishedAt: publishedAt, Summary: summary, Content: content,
		RawEntry: map[string]any{"title": title, "link": link, "external_id": externalID, "author": author, "published_at": publishedAt},
	}
}

// parseFeedTime 解析常见 RSS 和 Atom 时间并输出 MySQL DATETIME 可写入的 UTC 文本。
// 输入：value 是 feed 时间文本。
// 输出：识别成功返回 YYYY-MM-DD HH:MM:SS，否则返回空字符串。
// 副作用：无。
func parseFeedTime(value string) string {
	// 1. 按常见格式顺序尝试解析。
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return ""
}

// htmlToText 把 feed HTML 压缩为纯文本。
// 输入：value 是可能包含实体和标签的文本。
// 输出：返回单空格分隔纯文本。
// 副作用：无。
func htmlToText(value string) string {
	// 1. 解码实体、移除标签并压缩空白。
	decoded := html.UnescapeString(value)
	withoutTags := htmlTagPattern.ReplaceAllString(decoded, " ")
	return strings.TrimSpace(spaceRunPattern.ReplaceAllString(withoutTags, " "))
}

// normalizeFeedText 解码实体并压缩连续空白。
// 输入：value 是 feed 文本。
// 输出：返回清理文本。
// 副作用：无。
func normalizeFeedText(value string) string {
	// 1. 统一标题、链接和作者清理规则。
	return strings.TrimSpace(spaceRunPattern.ReplaceAllString(html.UnescapeString(value), " "))
}

// responseMessage 从 WeChatRSS JSON 中读取可读说明。
// 输入：payload 是响应对象。
// 输出：返回顶层或 data.message，缺失时返回默认说明。
// 副作用：无。
func responseMessage(payload map[string]any) string {
	// 1. 优先读取顶层常见字段。
	for _, key := range []string{"message", "detail"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if value, ok := data["message"].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "上游未返回错误说明"
}

// firstNonEmpty 返回第一个非空字符串。
// 输入：values 是候选文本。
// 输出：返回首个非空值或空字符串。
// 副作用：无。
func firstNonEmpty(values ...string) string {
	// 1. 按调用方给定优先级查找。
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// truncateClientRunes 按 Unicode 字符截断客户端文本。
// 输入：value 是文本，limit 是最大字符数。
// 输出：返回合法 UTF-8 截断结果。
// 副作用：无。
func truncateClientRunes(value string, limit int) string {
	// 1. 短文本直接返回，长文本按 rune 截断。
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
