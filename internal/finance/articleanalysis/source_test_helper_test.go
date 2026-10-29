package articleanalysis

import (
	"context"
	"database/sql"
)

// insertTestSource 创建同步测试使用的启用文章来源。
// 输入：db 是隔离测试库，feedURL 是来源地址。
// 输出：插入成功返回 nil。
// 副作用：写入 investment_article_source。
func insertTestSource(db *sql.DB, feedURL string) error {
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO investment_article_source (
			source_code, source_name, source_type, feed_url, weight, is_active, description
		) VALUES ('wechat_aggregate', '测试投资文章来源', 'feed', ?, 1, 1, '测试文章来源。')
		ON CONFLICT(source_code) DO UPDATE SET
			source_name = excluded.source_name,
			source_type = excluded.source_type,
			feed_url = excluded.feed_url,
			is_active = excluded.is_active,
			description = excluded.description
	`, feedURL)
	return err
}
