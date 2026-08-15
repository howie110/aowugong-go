package articleanalysis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const weReadFeedArticleLimit = 1000

var errWeReadFeedAccountNotFound = errors.New("微信读书公众号不存在")

// FeedSource 描述一个独立微信公众号 RSS 的稳定身份。
type FeedSource struct {
	AccountID string
	Title     string
}

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
// 输出：返回公众号身份及最多一千篇按发布时间倒序排列的文章；查询失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) WeReadFeedArticles(ctx context.Context, accountID string) (FeedSource, []FeedArticle, error) {
	// 1. 校验公众号仍在当前书架清单中，防止任意作者名被当作订阅源查询。
	source, err := s.repository.weReadFeedSource(ctx, accountID)
	if err != nil {
		return FeedSource{}, nil, fmt.Errorf("读取微信公众号 RSS 身份: %w", err)
	}

	// 2. 使用固定上限读取该公众号文章，避免 RSS 长期增长占用过多内存。
	articles, err := s.repository.weReadFeedArticles(ctx, source.Title, weReadFeedArticleLimit)
	if err != nil {
		return FeedSource{}, nil, fmt.Errorf("读取微信公众号 RSS 文章: %w", err)
	}
	return source, articles, nil
}

// IsWeReadFeedAccountNotFound 判断公众号 RSS 查询是否命中不存在的账号。
// 输入：err 是服务或仓储返回的包装错误。
// 输出：公众号不存在时返回 true。
// 副作用：无。
func IsWeReadFeedAccountNotFound(err error) bool {
	// 1. 沿错误链识别稳定哨兵错误。
	return errors.Is(err, errWeReadFeedAccountNotFound)
}

// weReadFeedSource 查询一个可输出 RSS 的微信读书公众号。
// 输入：ctx 控制查询，accountID 是微信读书书架公众号标识。
// 输出：返回公众号身份；不存在时返回稳定哨兵错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) weReadFeedSource(ctx context.Context, accountID string) (FeedSource, error) {
	// 1. 只接受当前保留的公众号，避免已从书架清单清理的来源继续暴露。
	var source FeedSource
	err := r.db.QueryRowContext(ctx, `
		SELECT account_id, title
		FROM weread_article_account
		WHERE account_id = ?
	`, accountID).Scan(&source.AccountID, &source.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return FeedSource{}, errWeReadFeedAccountNotFound
	}
	if err != nil {
		return FeedSource{}, fmt.Errorf("查询微信读书公众号 RSS 身份: %w", err)
	}
	return source, nil
}

// weReadFeedArticles 查询微信读书来源的最新文章。
// 输入：ctx 控制查询，limit 限制返回数量。
// 输出：返回文章元数据和正文；失败时返回带业务上下文的错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) weReadFeedArticles(ctx context.Context, author string, limit int) ([]FeedArticle, error) {
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
		WHERE source.source_type = 'weread' AND article.author = ?
		ORDER BY COALESCE(article.published_at, article.created_at) DESC, article.id DESC
		LIMIT ?
	`, author, limit)
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
