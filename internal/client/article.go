package client

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	htmlTagPattern  = regexp.MustCompile(`<[^>]+>`)
	spaceRunPattern = regexp.MustCompile(`\s+`)
)

// ArticleItem 描述外部文章客户端规范化后的一篇文章。
type ArticleItem struct {
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

// buildArticleKey 按来源和首个有效外部标识生成统一文章键。
// 输入：sourceID 是文章来源主键，identities 按稳定性从高到低排列。
// 输出：返回固定长度的 SHA-256 十六进制文章键。
// 副作用：无。
func buildArticleKey(sourceID int64, identities ...string) string {
	// 1. 使用唯一入口组合来源和外部标识并计算摘要。
	seed := strconv.FormatInt(sourceID, 10) + "|" + firstNonEmpty(identities...)
	hash := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(hash[:])
}

// parseFeedTime 解析外部文章时间并输出 SQLite DATETIME 可写入的上海时间文本。
// 输入：value 是外部 API 时间文本。
// 输出：识别成功返回 YYYY-MM-DD HH:MM:SS，否则返回空字符串。
// 副作用：无。
func parseFeedTime(value string) string {
	// 1. 按常见格式顺序尝试解析。
	value = strings.TrimSpace(value)
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.In(location).Format("2006-01-02 15:04:05")
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, location); err == nil {
		return parsed.Format("2006-01-02 15:04:05")
	}
	return ""
}

// htmlToText 把外部文章 HTML 压缩为纯文本。
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
// 输入：value 是外部文章文本。
// 输出：返回清理文本。
// 副作用：无。
func normalizeFeedText(value string) string {
	// 1. 统一标题、链接和作者清理规则。
	return strings.TrimSpace(spaceRunPattern.ReplaceAllString(html.UnescapeString(value), " "))
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
