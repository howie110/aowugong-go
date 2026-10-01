package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ArticleRepository 是网站文章、附件、RSS 和 Sitemap 的唯一读取来源。
type ArticleRepository struct{ db *sql.DB }

func NewArticleRepository(db *sql.DB) *ArticleRepository { return &ArticleRepository{db: db} }

const articleColumns = `slug, title, date, tags, html, description, toc`

func scanArticle(row interface{ Scan(...any) error }) (Article, error) {
	var a Article
	var tags, toc string
	if err := row.Scan(&a.Slug, &a.Title, &a.Date, &tags, &a.HTML, &a.Description, &toc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, ErrNotFound
		}
		return a, err
	}
	if err := json.Unmarshal([]byte(tags), &a.Tags); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(toc), &a.TOC); err != nil {
		return a, err
	}
	a.Thumbnail = firstBodyImage(a.HTML)
	return a, nil
}
func (r *ArticleRepository) Get(ctx context.Context, slug string) (Article, error) {
	return scanArticle(r.db.QueryRowContext(ctx, `SELECT `+articleColumns+` FROM blog_articles WHERE slug = ?`, slug))
}
func (r *ArticleRepository) List(ctx context.Context) ([]Article, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+articleColumns+` FROM blog_articles ORDER BY date DESC, slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Article{}
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

type ArticleAsset struct {
	Path        string
	Content     []byte
	ContentType string
}

func (r *ArticleRepository) Asset(ctx context.Context, path string) (ArticleAsset, error) {
	a := ArticleAsset{Path: path}
	err := r.db.QueryRowContext(ctx, `SELECT content,content_type FROM blog_article_assets WHERE path = ?`, path).Scan(&a.Content, &a.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return a, err
}

// replace 在一次事务中替换已验证的完整内容，序号更新同时串行化并发发布。
func (r *ArticleRepository) replace(ctx context.Context, articles []Article, assets []ArticleAsset, sequence int64) error {
	if sequence < 1 || len(articles) == 0 {
		return fmt.Errorf("发布序号和文章内容不能为空")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE blog_article_publication SET sequence = ? WHERE id = 1 AND sequence < ?`, sequence, sequence)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("拒绝旧的或重复的发布序号 %d", sequence)
	}
	for _, table := range []string{"blog_articles", "blog_article_assets"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
	}
	for _, a := range articles {
		tags, err := json.Marshal(a.Tags)
		if err != nil {
			return err
		}
		toc, err := json.Marshal(a.TOC)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO blog_articles (slug,markdown,title,date,tags,html,description,toc) VALUES (?,?,?,?,?,?,?,?)`, a.Slug, a.Markdown, a.Title, a.Date, string(tags), a.HTML, a.Description, string(toc))
		if err != nil {
			return err
		}
	}
	for _, a := range assets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO blog_article_assets(path,content,content_type) VALUES (?,?,?)`, a.Path, a.Content, a.ContentType); err != nil {
			return err
		}
	}
	return tx.Commit()
}
