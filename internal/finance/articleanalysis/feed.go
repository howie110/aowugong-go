package articleanalysis

import (
	"context"
	"fmt"
)

const weReadFeedArticleLimit = 1000

// FeedArticle 描述 RSS 输出所需的一篇微信读书公众号文章。
type FeedArticle struct {
	ID          int64
	ArticleKey  string
	Title       string
	Link        string
	Author      string
	PublishedAt string
	Summary     string
	Content     string
}

// WeReadFeedArticles 返回供本机 RSS 阅读器订阅的最新公众号文章。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回最多一千篇按发布时间倒序排列的文章；查询失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) WeReadFeedArticles(ctx context.Context) ([]FeedArticle, error) {
	// 1. 使用固定上限读取微信读书来源文章，避免 RSS 长期增长占用过多内存。
	articles, err := s.repository.weReadFeedArticles(ctx, weReadFeedArticleLimit)
	if err != nil {
		return nil, fmt.Errorf("读取微信公众号 RSS 文章: %w", err)
	}
	return articles, nil
}

// weReadFeedArticles 查询微信读书来源的最新文章。
// 输入：ctx 控制查询，limit 限制返回数量。
// 输出：返回文章元数据和正文；失败时返回带业务上下文的错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) weReadFeedArticles(ctx context.Context, limit int) ([]FeedArticle, error) {
	// 1. 限制查询规模并只读取微信读书来源，避免其他投资文章混入订阅源。
	if limit < 1 || limit > weReadFeedArticleLimit {
		limit = weReadFeedArticleLimit
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT article.id, article.article_key, article.title, article.link,
		       COALESCE(article.author, ''),
		       COALESCE(article.published_at, article.created_at, ''),
		       COALESCE(article.summary, ''), COALESCE(article.content, '')
		FROM investment_article article
		JOIN investment_article_source source ON source.id = article.source_id
		WHERE source.source_type = 'weread'
		ORDER BY COALESCE(article.published_at, article.created_at) DESC, article.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询微信读书 RSS 文章: %w", err)
	}
	defer rows.Close()

	// 2. 扫描固定字段并保持数据库排序。
	articles := make([]FeedArticle, 0)
	for rows.Next() {
		var article FeedArticle
		if err := rows.Scan(
			&article.ID, &article.ArticleKey, &article.Title, &article.Link,
			&article.Author, &article.PublishedAt, &article.Summary, &article.Content,
		); err != nil {
			return nil, fmt.Errorf("扫描微信读书 RSS 文章: %w", err)
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历微信读书 RSS 文章: %w", err)
	}
	return articles, nil
}
