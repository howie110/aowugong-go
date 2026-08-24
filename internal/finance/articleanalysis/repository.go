package articleanalysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/howiedata/aowugong-go/internal/database"
)

// Repository 负责投资文章、来源和分析结果的 PostgreSQL 读写。
type Repository struct {
	db *sql.DB
}

type sourceRecord struct {
	Source
	FeedURL string
}

type pendingArticle struct {
	ID          int64
	Title       string
	Link        string
	Summary     string
	Content     string
	PublishedAt string
	SourceName  string
	SourceType  string
}

type pendingParseArticle struct {
	ID           int64
	Title        string
	Link         string
	RawEntryJSON string
}

type analysisRow struct {
	Recommendations []Signal
	Risks           []Signal
	MarketMood      string
	Prediction      string
	OccurredAt      string
}

type summaryCounts struct {
	SourceCount   int
	ArticleCount  int
	AnalyzedCount int
	PendingCount  int
	LatestAt      string
}

// NewRepository 创建投资文章 PostgreSQL 仓储。
// 输入：db 是已经执行版本化迁移的 PostgreSQL 连接。
// 输出：返回文章仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存显式数据库依赖。
	return &Repository{db: db}
}

// SignalGroups 读取信号榜使用的全部概念组和别名。
// 输入：ctx 控制数据库查询。
// 输出：按概念组和别名主键顺序返回完整映射；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) SignalGroups(ctx context.Context) ([]SignalGroup, error) {
	// 1. 联表读取至少包含一个别名的概念组。
	rows, err := r.db.QueryContext(ctx, `
		SELECT g.id, g.canonical_name, g.group_type, a.alias_name
		FROM investment_signal_group g
		JOIN investment_signal_alias a ON a.group_id = g.id
		ORDER BY g.id ASC, a.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("读取投资信号概念组: %w", err)
	}
	defer rows.Close()

	// 2. 按主键合并连续别名，同时保留数据库稳定顺序。
	positions := make(map[int64]int)
	result := make([]SignalGroup, 0)
	for rows.Next() {
		var id int64
		var name, kind, alias string
		if err := rows.Scan(&id, &name, &kind, &alias); err != nil {
			return nil, fmt.Errorf("读取投资信号概念组行: %w", err)
		}
		position, exists := positions[id]
		if !exists {
			position = len(result)
			positions[id] = position
			result = append(result, SignalGroup{ID: id, Name: name, Type: kind, Aliases: []string{}})
		}
		result[position].Aliases = append(result[position].Aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历投资信号概念组: %w", err)
	}
	return result, nil
}

// SaveSignalGroups 原子保存 DeepSeek 生成的概念组和原始名称映射。
// 输入：ctx 控制事务，groups 是已校验分类，modelName 是模型名称。
// 输出：返回本次新增别名数量；失败时回滚并返回错误。
// 副作用：写入 PostgreSQL investment_signal_group 和 investment_signal_alias。
func (r *Repository) SaveSignalGroups(ctx context.Context, groups []signalGroupProposal, modelName string) (int, error) {
	// 1. 开启事务，确保组和别名不会只写入一半。
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始保存投资信号分类: %w", err)
	}
	defer transaction.Rollback()
	inserted := 0

	// 2. 复用同名概念组主键，并用唯一约束防止并发重映射已有别名。
	for _, group := range groups {
		var groupID int64
		err := transaction.QueryRowContext(ctx, `
			INSERT INTO investment_signal_group(canonical_name, group_type, source, model_name)
			VALUES(?,?,?,?)
			ON CONFLICT(canonical_name) DO UPDATE SET
				updated_at = ?
			RETURNING id
		`, group.CanonicalName, group.Type, "deepseek", modelName,
			appdatabase.TimestampText(time.Now())).Scan(&groupID)
		if err != nil {
			return 0, fmt.Errorf("保存投资信号概念组 %s: %w", group.CanonicalName, err)
		}
		for _, alias := range group.Aliases {
			aliasResult, err := transaction.ExecContext(ctx, `
				INSERT INTO investment_signal_alias(
					group_id, alias_name, normalized_name, confidence, source, model_name
				) VALUES(?,?,?,?,?,?)
				ON CONFLICT(normalized_name) DO NOTHING
			`, groupID, alias.Name, normalizeSignalAlias(alias.Name), alias.Confidence, "deepseek", modelName)
			if err != nil {
				return 0, fmt.Errorf("保存投资信号别名 %s: %w", alias.Name, err)
			}
			affected, err := aliasResult.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf("读取投资信号别名 %s 写入结果: %w", alias.Name, err)
			}
			inserted += int(affected)
		}
	}

	// 3. 全部写入成功后提交事务并返回新增数量。
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("提交投资信号分类: %w", err)
	}
	return inserted, nil
}

// ReplaceSignalGroups 在单个事务中整体替换投资信号概念词典。
// 输入：ctx 控制事务，groups 是已全局校验的新词典，modelName 记录模型版本。
// 输出：返回写入别名数量；清理、写入或提交失败时回滚并返回错误。
// 副作用：删除并重建 PostgreSQL investment_signal_group 和 investment_signal_alias。
func (r *Repository) ReplaceSignalGroups(ctx context.Context, groups []signalGroupProposal, modelName string) (int, error) {
	// 1. 拒绝空词典并开启覆盖事务，防止线上映射被意外清空。
	if len(groups) == 0 {
		return 0, fmt.Errorf("拒绝使用空投资信号词典覆盖现有数据")
	}
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始重建投资信号词典: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `DELETE FROM investment_signal_alias`); err != nil {
		return 0, fmt.Errorf("清理旧投资信号别名: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM investment_signal_group`); err != nil {
		return 0, fmt.Errorf("清理旧投资信号概念组: %w", err)
	}

	// 2. 严格插入每个新组及其唯一别名，任何冲突都让整个事务失败。
	inserted := 0
	for _, group := range groups {
		var groupID int64
		err := transaction.QueryRowContext(ctx, `
			INSERT INTO investment_signal_group(canonical_name, group_type, source, model_name)
			VALUES(?,?,?,?)
			RETURNING id
		`, group.CanonicalName, group.Type, "deepseek_rebuild", modelName).Scan(&groupID)
		if err != nil {
			return 0, fmt.Errorf("重建投资信号概念组 %s: %w", group.CanonicalName, err)
		}
		for _, alias := range group.Aliases {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO investment_signal_alias(
					group_id, alias_name, normalized_name, confidence, source, model_name
				) VALUES(?,?,?,?,?,?)
			`, groupID, alias.Name, normalizeSignalAlias(alias.Name), alias.Confidence, "deepseek_rebuild", modelName); err != nil {
				return 0, fmt.Errorf("重建投资信号别名 %s: %w", alias.Name, err)
			}
			inserted++
		}
	}

	// 3. 全量词典写入成功后一次提交。
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("提交投资信号词典重建: %w", err)
	}
	return inserted, nil
}

// SyncDefaultSource 初始化当前唯一的 Miniflux 投资文章来源。
// 输入：ctx 控制数据库操作，feedURL 是 Miniflux 根地址。
// 输出：成功返回 nil，失败返回错误。
// 副作用：写入 investment_article_source；启用时会把旧 WeChatRSS 元数据切换为 Miniflux。
func (r *Repository) SyncDefaultSource(ctx context.Context, feedURL string) error {
	// 1. 根据 Miniflux 地址决定默认来源是否启用。
	feedURL = strings.TrimSpace(feedURL)
	active := 0
	status := "missing_url"
	message := "未配置 MINIFLUX_API_TOKEN"
	if feedURL != "" {
		active = 1
		status = "ready"
		message = ""
	}

	// 2. 空地址只补建缺失来源，避免覆盖线上已经可用的地址。
	if feedURL == "" {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO investment_article_source (
				source_code, source_name, source_type, feed_url, is_active,
				description, last_fetch_status, last_fetch_message
			) VALUES ('wechat_aggregate', 'Miniflux 投资文章', 'miniflux', ?, ?, 'Miniflux 投资文章分类。', ?, ?)
			ON CONFLICT(source_code) DO NOTHING
		`, feedURL, active, status, nullableArticleText(message))
		if err != nil {
			return fmt.Errorf("补建默认文章来源: %w", err)
		}
		return nil
	}

	// 3. 有效地址覆盖旧抓取端点，但复用原来源代码和主键保留文章关联。
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO investment_article_source (
			source_code, source_name, source_type, feed_url, is_active,
			description, last_fetch_status, last_fetch_message
		) VALUES ('wechat_aggregate', 'Miniflux 投资文章', 'miniflux', ?, ?, 'Miniflux 投资文章分类。', ?, ?)
		ON CONFLICT(source_code) DO UPDATE SET
			source_name = 'Miniflux 投资文章',
			source_type = 'miniflux',
			feed_url = excluded.feed_url,
			is_active = excluded.is_active,
			description = 'Miniflux 投资文章分类。',
			updated_at = ?
	`, feedURL, active, status, nullableArticleText(message), appdatabase.TimestampText(time.Now()))
	if err != nil {
		return fmt.Errorf("同步默认文章来源: %w", err)
	}
	return nil
}

// Sources 列出文章来源。
// 输入：ctx 控制查询，activeOnly 控制是否只返回启用且有地址的来源。
// 输出：按主键正序返回来源；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) Sources(ctx context.Context, activeOnly bool) ([]Source, error) {
	// 1. 构造固定查询条件并执行。
	query := `
		SELECT id, source_code, source_name, source_type, feed_url, is_active,
		       COALESCE(description, ''), COALESCE(last_fetch_at, ''),
		       COALESCE(last_fetch_status, ''), COALESCE(last_fetch_message, '')
		FROM investment_article_source`
	if activeOnly {
		query += " WHERE is_active = 1 AND feed_url <> ''"
	}
	query += " ORDER BY id ASC"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询文章来源: %w", err)
	}
	defer rows.Close()

	// 2. 扫描来源并隐藏 API 不需要暴露的 feed URL。
	results := make([]Source, 0)
	for rows.Next() {
		var item Source
		var feedURL string
		var active int
		if err := rows.Scan(
			&item.ID, &item.SourceCode, &item.SourceName, &item.SourceType, &feedURL, &active,
			&item.Description, &item.LastFetchAt, &item.LastFetchStatus, &item.LastFetchMessage,
		); err != nil {
			return nil, fmt.Errorf("扫描文章来源: %w", err)
		}
		item.FeedURL = feedURL
		item.IsActive = active == 1
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历文章来源: %w", err)
	}
	return results, nil
}

// sourceRecords 返回同步任务需要的启用来源及其内部地址。
// 输入：ctx 控制查询。
// 输出：返回内部来源记录；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) sourceRecords(ctx context.Context) ([]sourceRecord, error) {
	// 1. 复用来源查询并保留模型中的内部 FeedURL 字段。
	sources, err := r.Sources(ctx, true)
	if err != nil {
		return nil, err
	}
	results := make([]sourceRecord, 0, len(sources))
	for _, source := range sources {
		results = append(results, sourceRecord{Source: source, FeedURL: source.FeedURL})
	}
	return results, nil
}

// UpsertArticle 按稳定文章键或原文链接新增外部文章，已有文章直接返回。
// 输入：ctx 控制事务，sourceID 是来源，entry 是规范化文章。
// 输出：返回 inserted 或 unchanged 及文章主键；失败时返回错误。
// 副作用：仅在文章尚未入库时写入 investment_article。
func (r *Repository) UpsertArticle(ctx context.Context, sourceID int64, entry FeedEntry) (string, int64, error) {
	// 1. 查询稳定文章键，已有记录直接返回，避免重复更新正文大字段。
	var existingID int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM investment_article WHERE article_key = ?", entry.ArticleKey).Scan(&existingID)
	if err == nil {
		return "unchanged", existingID, nil
	}
	if err != sql.ErrNoRows {
		return "", 0, fmt.Errorf("查询现有投资文章: %w", err)
	}
	link := truncateRunes(strings.TrimSpace(entry.Link), 1000)
	if link != "" {
		err = r.db.QueryRowContext(ctx,
			"SELECT id FROM investment_article WHERE link = ? ORDER BY id ASC LIMIT 1", link).Scan(&existingID)
		if err == nil {
			return "unchanged", existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", 0, fmt.Errorf("按链接查询现有投资文章: %w", err)
		}
	}

	// 2. 稳定键和链接均未命中时，读取来源名称并准备新增文章字段。
	var sourceName string
	if err := r.db.QueryRowContext(ctx, "SELECT source_name FROM investment_article_source WHERE id = ?", sourceID).Scan(&sourceName); err != nil {
		return "", 0, fmt.Errorf("查询投资文章来源: %w", err)
	}
	author := strings.TrimSpace(entry.Author)
	if author == "" {
		author = sourceName
	}
	rawJSON, err := json.Marshal(entry.RawEntry)
	if err != nil {
		return "", 0, fmt.Errorf("序列化外部原始文章: %w", err)
	}

	// 3. 写入新文章，并在极小概率并发冲突时通过唯一键保留稳定主键。
	externalID := nullableArticleText(truncateRunes(entry.ExternalID, 255))
	title := fallbackTitle(truncateRunes(entry.Title, 480))
	author = truncateRunes(author, 100)
	publishedAt := nullableArticleText(entry.PublishedAt)
	summary := nullableArticleText(entry.Summary)
	content := nullableArticleText(entry.Content)
	var articleID int64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO investment_article (
			source_id, article_key, external_id, title, link, author,
			published_at, summary, content, raw_entry_json, fetch_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(article_key) DO NOTHING
		RETURNING id
	`, sourceID, entry.ArticleKey, externalID, title, link, author, publishedAt, summary, content, string(rawJSON), articleFetchStatus(entry.FetchStatus)).Scan(&articleID)
	if err == sql.ErrNoRows {
		if err := r.db.QueryRowContext(ctx,
			"SELECT id FROM investment_article WHERE article_key = ?", entry.ArticleKey).Scan(&articleID); err != nil {
			return "", 0, fmt.Errorf("读取并发写入投资文章主键: %w", err)
		}
		return "unchanged", articleID, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("写入投资文章: %w", err)
	}
	return "inserted", articleID, nil
}

// pendingParseArticles 读取尚未成功获取正文的文章。
// 输入：ctx 控制查询，limit 限制本次解析数量。
// 输出：返回待解析文章；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) pendingParseArticles(ctx context.Context, limit int) ([]pendingParseArticle, error) {
	// 1. 只选择元数据已入库但正文仍待解析的文章。
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, link, COALESCE(raw_entry_json, '') FROM investment_article
		WHERE fetch_status = 'pending_parse' AND COALESCE(link, '') <> ''
		ORDER BY COALESCE(published_at, created_at) DESC, id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询待解析投资文章: %w", err)
	}
	defer rows.Close()
	result := make([]pendingParseArticle, 0)
	for rows.Next() {
		var item pendingParseArticle
		if err := rows.Scan(&item.ID, &item.Title, &item.Link, &item.RawEntryJSON); err != nil {
			return nil, fmt.Errorf("扫描待解析投资文章: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待解析投资文章: %w", err)
	}
	return result, nil
}

// UpdateArticleContent 保存文章正文并更新解析状态。
// 输入：ctx 控制写入，articleID 标识文章，content 是正文，status 是 parsed。
// 输出：返回写入错误。
// 副作用：写入 PostgreSQL investment_article。
func (r *Repository) UpdateArticleContent(ctx context.Context, articleID int64, content, summary, status string) error {
	// 1. 只有非空正文才允许标记为已解析。
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("文章正文为空")
	}
	if status != "parsed" {
		status = "pending_parse"
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE investment_article SET content = ?, summary = ?, fetch_status = ?, updated_at = ? WHERE id = ?
	`, content, nullableArticleText(summary), status, appdatabase.TimestampText(time.Now()), articleID)
	if err != nil {
		return fmt.Errorf("保存文章正文: %w", err)
	}
	return nil
}

func articleFetchStatus(value string) string {
	if value == "pending_parse" {
		return value
	}
	return "parsed"
}

// UpdateSourceStatus 更新信息源最近抓取结果。
// 输入：ctx 控制写入，sourceID 标识来源，status 和 message 描述结果。
// 输出：成功返回 nil，失败返回错误。
// 副作用：写入 investment_article_source。
func (r *Repository) UpdateSourceStatus(ctx context.Context, sourceID int64, status, message string) error {
	// 1. 写入上海时区时间和截断后的可读消息。
	_, err := r.db.ExecContext(ctx, `
		UPDATE investment_article_source
		SET last_fetch_at = ?, last_fetch_status = ?, last_fetch_message = ?,
		    updated_at = ?
		WHERE id = ?
	`, shanghaiNowText(), status, nullableArticleText(truncateRunes(message, 500)),
		appdatabase.TimestampText(time.Now()), sourceID)
	if err != nil {
		return fmt.Errorf("更新文章来源抓取状态: %w", err)
	}
	return nil
}

// Articles 读取指定天数内分析成功的文章列表。
// 输入：ctx 控制查询，days 是日期范围，limit 是 1 到 5000 的上限。
// 输出：按业务日期倒序返回文章；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) Articles(ctx context.Context, days, limit int) ([]ArticleItem, error) {
	// 1. 限制查询范围并按 PostgreSQL DATETIME 比较业务日期。
	if days < 1 {
		days = DefaultTargetDays
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Format("2006-01-02 15:04:05")
	rows, err := r.db.QueryContext(ctx, `
		SELECT article.id, source.source_name, article.title, COALESCE(article.author, ''),
		       COALESCE(article.published_at, ''), COALESCE(article.created_at, ''),
		       COALESCE(analysis.market_mood, ''), COALESCE(analysis.market_prediction, ''),
		       COALESCE(analysis.recommendations_json, '[]'), COALESCE(analysis.risks_json, '[]')
		FROM investment_article article
		JOIN investment_article_source source ON source.id = article.source_id
		JOIN investment_article_analysis analysis ON analysis.article_id = article.id AND analysis.status = 'success'
		WHERE COALESCE(article.published_at, article.created_at) >= ?
		ORDER BY COALESCE(article.published_at, article.created_at) DESC, article.id DESC
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("查询投资文章列表: %w", err)
	}
	defer rows.Close()

	// 2. 扫描文章和两个信号 JSON 数组。
	results := make([]ArticleItem, 0)
	for rows.Next() {
		item, err := scanArticleItem(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描投资文章列表: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历投资文章列表: %w", err)
	}
	return results, nil
}

// Detail 按主键读取文章和完整分析结果。
// 输入：ctx 控制查询，articleID 是文章主键。
// 输出：存在时返回详情，不存在时返回 nil；查询失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) Detail(ctx context.Context, articleID int64) (*ArticleDetail, error) {
	// 1. 一次联表读取文章、列表字段和完整分析字段。
	row := r.db.QueryRowContext(ctx, `
		SELECT article.id, source.source_name, article.title, COALESCE(article.author, ''),
		       COALESCE(article.published_at, ''), COALESCE(article.created_at, ''),
		       COALESCE(analysis.market_mood, ''), COALESCE(analysis.market_prediction, ''),
		       COALESCE(analysis.recommendations_json, '[]'), COALESCE(analysis.risks_json, '[]'),
		       article.link, COALESCE(article.prompt_feedback, ''),
		       analysis.id, COALESCE(analysis.summary, ''), COALESCE(analysis.market_mood_reason, ''),
		       COALESCE(analysis.market_prediction_reason, ''), COALESCE(analysis.error_message, '')
		FROM investment_article article
		JOIN investment_article_source source ON source.id = article.source_id
		LEFT JOIN investment_article_analysis analysis ON analysis.article_id = article.id
		WHERE article.id = ?
	`, articleID)
	var item ArticleItem
	var publishedAt, createdAt, recommendationsJSON, risksJSON string
	var detail ArticleDetail
	var analysisID sql.NullInt64
	var analysisSummary, moodReason, predictionReason, errorMessage string
	err := row.Scan(
		&item.ID, &item.SourceName, &item.Title, &item.Author, &publishedAt, &createdAt,
		&item.MarketMood, &item.MarketPrediction, &recommendationsJSON, &risksJSON,
		&detail.Link, &detail.PromptFeedback, &analysisID, &analysisSummary, &moodReason, &predictionReason, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询投资文章详情: %w", err)
	}

	// 2. 解码信号并组装嵌套详情。
	recommendations, err := decodeSignals(recommendationsJSON)
	if err != nil {
		return nil, fmt.Errorf("解析文章推荐信号: %w", err)
	}
	risks, err := decodeSignals(risksJSON)
	if err != nil {
		return nil, fmt.Errorf("解析文章风险信号: %w", err)
	}
	item.PublishedAt = dateOnly(publishedAt)
	item.CreatedAt = dateOnly(createdAt)
	item.RecommendationNames = signalNames(recommendations)
	item.RiskNames = signalNames(risks)
	detail.ArticleItem = item
	if analysisID.Valid {
		detail.Analysis = &Analysis{
			Summary: analysisSummary, MarketMood: item.MarketMood, MarketMoodReason: moodReason,
			MarketPrediction: item.MarketPrediction, MarketPredictionReason: predictionReason,
			Recommendations: recommendations, Risks: risks, ErrorMessage: errorMessage,
		}
	}
	return &detail, nil
}

// UpdatePromptFeedback 更新管理员对单篇文章的提示词修正意见。
// 输入：ctx 控制写入，articleID 是文章主键，feedback 是最多 4000 字意见。
// 输出：返回更新后的详情；文章不存在时返回 nil。
// 副作用：写入 investment_article.prompt_feedback。
func (r *Repository) UpdatePromptFeedback(ctx context.Context, articleID int64, feedback string) (*ArticleDetail, error) {
	// 1. 更新截断后的可空反馈并检查文章是否存在。
	result, err := r.db.ExecContext(ctx, `
		UPDATE investment_article
		SET prompt_feedback = ?, updated_at = ?
		WHERE id = ?
	`, nullableArticleText(truncateRunes(strings.TrimSpace(feedback), 4000)),
		appdatabase.TimestampText(time.Now()), articleID)
	if err != nil {
		return nil, fmt.Errorf("更新文章提示词反馈: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("读取文章反馈更新数量: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}

	// 2. 返回统一详情模型供抽屉原地刷新。
	return r.Detail(ctx, articleID)
}

// pendingArticles 读取仍需模型分析的最近文章。
// 输入：ctx 控制查询，limit 是 1 到 50 的上限。
// 输出：返回内部待分析文章；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) pendingArticles(ctx context.Context, limit int) ([]pendingArticle, error) {
	// 1. 限制批量分析规模并读取未成功记录。
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT article.id, article.title, article.link, COALESCE(article.summary, ''),
		       COALESCE(article.content, ''), COALESCE(article.published_at, ''),
		       source.source_name, source.source_type
		FROM investment_article article
		JOIN investment_article_source source ON source.id = article.source_id
		LEFT JOIN investment_article_analysis analysis ON analysis.article_id = article.id
		WHERE article.fetch_status <> 'pending_parse'
		  AND (analysis.id IS NULL OR analysis.status != 'success')
		ORDER BY COALESCE(article.published_at, article.created_at) DESC, article.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询待分析投资文章: %w", err)
	}
	defer rows.Close()
	results := make([]pendingArticle, 0)
	for rows.Next() {
		var item pendingArticle
		if err := rows.Scan(&item.ID, &item.Title, &item.Link, &item.Summary, &item.Content, &item.PublishedAt, &item.SourceName, &item.SourceType); err != nil {
			return nil, fmt.Errorf("扫描待分析投资文章: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待分析投资文章: %w", err)
	}
	return results, nil
}

// SaveAnalysis 新增或覆盖单篇文章的模型分析结果。
// 输入：ctx 控制写入，articleID 标识文章，status/result/errorMessage 描述结果，model 和 promptVersion 记录版本。
// 输出：成功返回 nil，失败返回错误。
// 副作用：写入 investment_article_analysis。
func (r *Repository) SaveAnalysis(ctx context.Context, articleID int64, status string, result AnalysisResult, errorMessage, model, promptVersion string) error {
	// 1. 序列化信号和完整原始结果。
	recommendationsJSON, err := json.Marshal(result.Recommendations)
	if err != nil {
		return fmt.Errorf("序列化推荐信号: %w", err)
	}
	risksJSON, err := json.Marshal(result.Risks)
	if err != nil {
		return fmt.Errorf("序列化风险信号: %w", err)
	}
	rawJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化文章分析结果: %w", err)
	}
	analyzedAt := any(nil)
	if status == "success" {
		analyzedAt = shanghaiNowText()
	}

	// 2. 使用文章唯一键 upsert 全部结构化字段。
	values := []any{
		status, nullableArticleText(model), nullableArticleText(promptVersion), nullableArticleText(result.Summary),
		nullableArticleText(result.Market.Mood), nullableArticleText(result.Market.MoodReason),
		nullableArticleText(result.Market.Prediction), nullableArticleText(result.Market.PredictionReason),
		string(recommendationsJSON), string(risksJSON), string(rawJSON), nullableArticleText(truncateRunes(errorMessage, 1000)), analyzedAt,
	}
	arguments := append([]any{articleID}, values...)
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO investment_article_analysis (
			article_id, status, model_name, prompt_version, summary,
			market_mood, market_mood_reason, market_prediction, market_prediction_reason,
			recommendations_json, risks_json, raw_result_json, error_message, analyzed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(article_id) DO UPDATE SET
			status = excluded.status,
			model_name = excluded.model_name,
			prompt_version = excluded.prompt_version,
			summary = excluded.summary,
			market_mood = excluded.market_mood,
			market_mood_reason = excluded.market_mood_reason,
			market_prediction = excluded.market_prediction,
			market_prediction_reason = excluded.market_prediction_reason,
			recommendations_json = excluded.recommendations_json,
			risks_json = excluded.risks_json,
			raw_result_json = excluded.raw_result_json,
			error_message = excluded.error_message,
			analyzed_at = excluded.analyzed_at,
			updated_at = ?
	`, append(arguments, appdatabase.TimestampText(time.Now()))...)
	if err != nil {
		return fmt.Errorf("保存投资文章分析结果: %w", err)
	}
	return nil
}

// analysisRows 读取指定天数内分析成功的统计原始行。
// 输入：ctx 控制查询，days 是至少一天的范围。
// 输出：返回信号和市场枚举行；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) analysisRows(ctx context.Context, days int) ([]analysisRow, error) {
	// 1. 使用业务日期范围限制分析查询。
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Format("2006-01-02 15:04:05")
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(analysis.recommendations_json, '[]'), COALESCE(analysis.risks_json, '[]'),
		       COALESCE(analysis.market_mood, 'unknown'), COALESCE(analysis.market_prediction, 'unknown'),
		       COALESCE(article.published_at, analysis.analyzed_at, article.created_at)
		FROM investment_article_analysis analysis
		JOIN investment_article article ON article.id = analysis.article_id
		WHERE analysis.status = 'success'
		  AND COALESCE(article.published_at, article.created_at) >= ?
		ORDER BY COALESCE(article.published_at, article.created_at) DESC, article.id DESC
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("查询文章分析统计行: %w", err)
	}
	defer rows.Close()

	// 2. 解码每行信号 JSON。
	results := make([]analysisRow, 0)
	for rows.Next() {
		var item analysisRow
		var recommendationsJSON, risksJSON string
		if err := rows.Scan(&recommendationsJSON, &risksJSON, &item.MarketMood, &item.Prediction, &item.OccurredAt); err != nil {
			return nil, fmt.Errorf("扫描文章分析统计行: %w", err)
		}
		item.Recommendations, err = decodeSignals(recommendationsJSON)
		if err != nil {
			return nil, fmt.Errorf("解析统计推荐信号: %w", err)
		}
		item.Risks, err = decodeSignals(risksJSON)
		if err != nil {
			return nil, fmt.Errorf("解析统计风险信号: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历文章分析统计行: %w", err)
	}
	return results, nil
}

// counts 读取文章页面顶部使用的轻量计数。
// 输入：ctx 控制查询。
// 输出：返回来源、文章、已分析、待分析和最新日期；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) counts(ctx context.Context) (summaryCounts, error) {
	// 1. 使用单条聚合查询读取全部计数。
	var result summaryCounts
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM investment_article_source WHERE is_active = 1),
			(SELECT COUNT(*) FROM investment_article),
			(SELECT COUNT(*) FROM investment_article_analysis WHERE status = 'success'),
			(SELECT COUNT(*) FROM investment_article article
			 LEFT JOIN investment_article_analysis analysis ON analysis.article_id = article.id
			 WHERE analysis.id IS NULL OR analysis.status != 'success'),
			COALESCE((SELECT MAX(COALESCE(published_at, created_at)) FROM investment_article), '')
	`).Scan(&result.SourceCount, &result.ArticleCount, &result.AnalyzedCount, &result.PendingCount, &result.LatestAt)
	if err != nil {
		return summaryCounts{}, fmt.Errorf("查询文章摘要计数: %w", err)
	}
	result.LatestAt = dateOnly(result.LatestAt)
	return result, nil
}

type articleRowScanner interface {
	Scan(dest ...any) error
}

// scanArticleItem 扫描文章列表行并展开信号名称。
// 输入：scanner 是 Rows 行扫描器。
// 输出：返回文章列表模型；JSON 无效时返回错误。
// 副作用：无。
func scanArticleItem(scanner articleRowScanner) (ArticleItem, error) {
	// 1. 扫描基础字段和两个信号 JSON。
	var item ArticleItem
	var publishedAt, createdAt, recommendationsJSON, risksJSON string
	if err := scanner.Scan(
		&item.ID, &item.SourceName, &item.Title, &item.Author, &publishedAt, &createdAt,
		&item.MarketMood, &item.MarketPrediction, &recommendationsJSON, &risksJSON,
	); err != nil {
		return ArticleItem{}, err
	}
	recommendations, err := decodeSignals(recommendationsJSON)
	if err != nil {
		return ArticleItem{}, err
	}
	risks, err := decodeSignals(risksJSON)
	if err != nil {
		return ArticleItem{}, err
	}

	// 2. API 日期只输出年月日，作者仍保留数据库全名。
	item.PublishedAt = dateOnly(publishedAt)
	item.CreatedAt = dateOnly(createdAt)
	item.RecommendationNames = signalNames(recommendations)
	item.RiskNames = signalNames(risks)
	return item, nil
}

// decodeSignals 解码数据库中的信号 JSON 数组。
// 输入：value 是 JSON 文本。
// 输出：返回非 nil 信号数组；无效 JSON 时返回错误。
// 副作用：无。
func decodeSignals(value string) ([]Signal, error) {
	// 1. 空值按空数组处理。
	if strings.TrimSpace(value) == "" {
		return []Signal{}, nil
	}
	var results []Signal
	if err := json.Unmarshal([]byte(value), &results); err != nil {
		return nil, err
	}
	if results == nil {
		results = []Signal{}
	}
	return results, nil
}

// signalNames 提取信号名称供文章列表筛选。
// 输入：items 是完整信号。
// 输出：返回非空名称数组。
// 副作用：无。
func signalNames(items []Signal) []string {
	// 1. 保持信号原始顺序并过滤空名称。
	results := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			results = append(results, item.Name)
		}
	}
	return results
}

// fallbackTitle 给空文章标题提供稳定占位值。
// 输入：value 是清理后的标题。
// 输出：非空返回原值，空值返回“未命名文章”。
// 副作用：无。
func fallbackTitle(value string) string {
	// 1. 防止数据库非空字段写入空标题。
	if strings.TrimSpace(value) == "" {
		return "未命名文章"
	}
	return value
}

// nullableArticleText 把空文章文本转换为 PostgreSQL NULL。
// 输入：value 是待写入文本。
// 输出：空白返回 nil，否则返回原文本。
// 副作用：无。
func nullableArticleText(value string) any {
	// 1. 统一文章可空字段转换。
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// dateOnly 把数据库时间统一转换为 YYYY-MM-DD。
// 输入：value 是 RFC3339 或常见数据库时间文本。
// 输出：能识别时返回日期，空值返回空字符串。
// 副作用：无。
func dateOnly(value string) string {
	// 1. 数据迁移后的时间格式都以 ISO 日期开头，安全截取前十位。
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

// shanghaiNowText 返回无时区歧义的上海本地时间文本。
// 输入：无。
// 输出：返回 YYYY-MM-DD HH:MM:SS。
// 副作用：读取系统时钟。
func shanghaiNowText() string {
	// 1. 中国标准时间全年固定 UTC+8。
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return time.Now().In(location).Format("2006-01-02 15:04:05")
}
