package articleanalysis

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestRepositorySignalGroupsReturnsAllAliases 验证概念组读取不会省略任何原始名称。
// 输入：同一概念组关联三个别名的模拟查询结果。
// 输出：返回一个“证券行业”组及完整三个别名。
// 副作用：执行模拟 SQLite 查询。
func TestRepositorySignalGroupsReturnsAllAliases(t *testing.T) {
	// 1. 准备按概念组和别名主键排序的数据库行。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT g.id, g.canonical_name, g.group_type, a.alias_name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "canonical_name", "group_type", "alias_name"}).
			AddRow(7, "证券行业", "sector", "券商").
			AddRow(7, "证券行业", "sector", "券商板块").
			AddRow(7, "证券行业", "sector", "中信证券"))

	// 2. 读取并核对组名、类型和全部别名顺序。
	groups, err := NewRepository(db).SignalGroups(context.Background())
	if err != nil {
		t.Fatalf("SignalGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].ID != 7 || groups[0].Name != "证券行业" || groups[0].Type != "sector" {
		t.Fatalf("groups = %#v", groups)
	}
	if strings.Join(groups[0].Aliases, ",") != "券商,券商板块,中信证券" {
		t.Fatalf("aliases = %#v", groups[0].Aliases)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestRepositorySaveSignalGroupsWritesGroupAndAliasesInOneTransaction 验证分类结果原子写入概念组和别名。
// 输入：一个“证券行业”组及两个高置信度别名。
// 输出：返回两条新增别名，事务完整提交。
// 副作用：执行模拟 SQLite 事务写入。
func TestRepositorySaveSignalGroupsWritesGroupAndAliasesInOneTransaction(t *testing.T) {
	// 1. 声明概念组、别名写入和事务提交预期。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO investment_signal_group").
		WithArgs("证券行业", "sector", "deepseek", "test-model", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "券商", "券商", 0.98, "deepseek", "test-model", pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "中信证券", "中信证券", 0.95, "deepseek", "test-model", pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec("DELETE FROM investment_signal_group").
		WithArgs(pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	// 2. 保存分类并核对新增别名数量和全部 SQL 预期。
	groups := []signalGroupProposal{{
		CanonicalName: "证券行业", Type: "sector",
		Aliases: []signalAliasProposal{{Name: "券商", Confidence: 0.98}, {Name: "中信证券", Confidence: 0.95}},
	}}
	inserted, err := NewRepository(db).SaveSignalGroups(context.Background(), groups, "test-model")
	if err != nil {
		t.Fatalf("SaveSignalGroups() error = %v", err)
	}
	if inserted != 2 {
		t.Fatalf("inserted = %d, want 2", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestRepositorySaveSignalGroupsMovesPendingAliasAndDeletesEmptyGroup 验证历史模糊映射只能迁移到正式组。
func TestRepositorySaveSignalGroupsMovesPendingAliasAndDeletesEmptyGroup(t *testing.T) {
	ctx := context.Background()
	db := testdatabase.Open(t)
	var pendingGroupID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO investment_signal_group(canonical_name, group_type, source)
		VALUES(?,?,?) RETURNING id
	`, pendingSignalGroupName, pendingSignalGroupType, "legacy").Scan(&pendingGroupID); err != nil {
		t.Fatalf("insert pending group: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO investment_signal_alias(group_id, alias_name, normalized_name, confidence, source)
		VALUES(?,?,?,?,?)
	`, pendingGroupID, "车企L", "车企l", 0.2, "legacy"); err != nil {
		t.Fatalf("insert pending alias: %v", err)
	}

	count, err := NewRepository(db).SaveSignalGroups(ctx, []signalGroupProposal{{
		CanonicalName: "汽车行业", Type: "sector",
		Aliases: []signalAliasProposal{{Name: "车企L", Confidence: 0.6}},
	}}, "test-model")
	if err != nil || count != 1 {
		t.Fatalf("SaveSignalGroups() = %d, %v", count, err)
	}
	var groupName string
	if err := db.QueryRowContext(ctx, `
		SELECT g.canonical_name
		FROM investment_signal_alias a JOIN investment_signal_group g ON g.id=a.group_id
		WHERE a.normalized_name=?
	`, "车企l").Scan(&groupName); err != nil || groupName != "汽车行业" {
		t.Fatalf("moved group = %q, %v", groupName, err)
	}
	var pendingCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM investment_signal_group
		WHERE canonical_name=? OR group_type=?
	`, pendingSignalGroupName, pendingSignalGroupType).Scan(&pendingCount); err != nil || pendingCount != 0 {
		t.Fatalf("pending groups = %d, %v", pendingCount, err)
	}
}

// TestRepositoryArticlesAllowsFullSignalWindowLimit 验证文章筛选可读取完整六十天窗口。
// 输入：页面请求五千篇文章的上限。
// 输出：SQL 使用五千而不是旧的两百上限。
// 副作用：执行模拟 SQLite 查询。
func TestRepositoryArticlesAllowsFullSignalWindowLimit(t *testing.T) {
	// 1. 要求文章查询携带日期范围和五千条限制。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT article.id, source.source_name, article.title").
		WithArgs(sqlmock.AnyArg(), 5000).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_name", "title", "author", "published_at", "created_at",
			"market_mood", "market_prediction", "recommendations_json", "risks_json",
		}))

	// 2. 执行查询并核对仓储没有把上限压回两百。
	articles, err := NewRepository(db).Articles(context.Background(), 60, 5000)
	if err != nil || len(articles) != 0 {
		t.Fatalf("Articles() = %#v, %v", articles, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestSyncDefaultSourceReplacesLegacyWeChatMetadata 验证启用 Miniflux 时复用原文章来源主键。
// 输入：一条已有文章关联的旧 WeChatRSS 来源和新的 Miniflux 根地址。
// 输出：来源地址、类型和说明切换为 Miniflux，原来源主键保持不变。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestSyncDefaultSourceReplacesLegacyWeChatMetadata(t *testing.T) {
	// 1. 写入旧 WeChatRSS 来源，模拟线上已有文章引用的稳定来源。
	ctx := context.Background()
	db := testdatabase.Open(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO investment_article_source(
		source_code, source_name, source_type, feed_url, weight, is_active, description
	) VALUES('wechat_aggregate','公众号聚合','wechat_rss_aggregate','http://127.0.0.1:5000/api/rss/all',1,1,'微信公众号聚合 RSS。')`); err != nil {
		t.Fatalf("insert legacy source: %v", err)
	}
	var originalID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM investment_article_source WHERE source_code = 'wechat_aggregate'`).Scan(&originalID); err != nil {
		t.Fatalf("query legacy source: %v", err)
	}

	// 2. 切换来源并核对原主键上的元数据被完整替换。
	if err := NewRepository(db).SyncDefaultSource(ctx, "http://127.0.0.1:5000"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	var id int64
	var name, sourceType, feedURL, description string
	var active int
	if err := db.QueryRowContext(ctx, `SELECT id, source_name, source_type, feed_url, is_active, description
		FROM investment_article_source WHERE source_code = 'wechat_aggregate'`).
		Scan(&id, &name, &sourceType, &feedURL, &active, &description); err != nil {
		t.Fatalf("query switched source: %v", err)
	}
	if id != originalID || name != "Miniflux 投资文章" || sourceType != "miniflux" ||
		feedURL != "http://127.0.0.1:5000" || active != 1 || description != "Miniflux 投资文章分类。" {
		t.Fatalf("source = id:%d name:%q type:%q url:%q active:%d description:%q",
			id, name, sourceType, feedURL, active, description)
	}
}

// TestRepositorySkipsExistingArticle 验证已入库文章不会再次写入大字段。
// 输入：数据库已存在同一 article_key 的文章。
// 输出：返回 unchanged 和原文章主键。
// 副作用：仅执行一条模拟查询，不执行 INSERT 或 UPDATE。
func TestRepositorySkipsExistingArticle(t *testing.T) {
	// 1. 创建只期望读取稳定文章键的模拟数据库。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM investment_article WHERE article_key = ?")).
		WithArgs("existing-key").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// 2. 重复写入时必须直接返回，不再查询来源或更新文章正文。
	action, articleID, err := NewRepository(db).UpsertArticle(context.Background(), 1, FeedEntry{ArticleKey: "existing-key"})
	if err != nil || action != "unchanged" || articleID != 42 {
		t.Fatalf("UpsertArticle() = %q, %d, %v", action, articleID, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestRepositorySkipsArticleWithExistingLink 验证切换文章来源后按原链接复用历史文章。
// 输入：旧文章键和新 Miniflux 文章键使用同一个文章 URL。
// 输出：第二次写入返回 unchanged 和旧文章主键，表内仍只有一篇文章。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestRepositorySkipsArticleWithExistingLink(t *testing.T) {
	// 1. 创建默认来源并写入旧文章键。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	sources, err := repository.Sources(ctx, true)
	if err != nil || len(sources) != 1 {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	firstAction, firstID, err := repository.UpsertArticle(ctx, sources[0].ID, FeedEntry{
		ArticleKey: "legacy-rss-key", Title: "历史文章", Link: "https://example.com/same-article",
	})
	if err != nil || firstAction != "inserted" {
		t.Fatalf("first UpsertArticle() = %q, %d, %v", firstAction, firstID, err)
	}

	// 2. 使用新的 Miniflux 文章键写入同一 URL，必须复用旧记录。
	secondAction, secondID, err := repository.UpsertArticle(ctx, sources[0].ID, FeedEntry{
		ArticleKey: "miniflux-key", Title: "历史文章", Link: "https://example.com/same-article",
	})
	if err != nil || secondAction != "unchanged" || secondID != firstID {
		t.Fatalf("second UpsertArticle() = %q, %d, %v, want unchanged/%d", secondAction, secondID, err, firstID)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM investment_article").Scan(&count); err != nil || count != 1 {
		t.Fatalf("article count = %d, %v, want 1", count, err)
	}
}

// TestRepositoryAndReportPreserveArticleContracts 验证文章存储、详情和 60/3 天报告契约。
// 输入：一篇当前文章和一条结构化分析结果。
// 输出：列表、详情、信号榜和市场分布均返回前端需要的字段。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestRepositoryAndReportPreserveArticleContracts(t *testing.T) {
	// 1. 创建完整迁移数据库并同步默认 Miniflux 来源。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	sources, err := repository.Sources(ctx, true)
	if err != nil || len(sources) != 1 {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}

	// 2. 写入文章和成功分析结果，作者保留全名。
	now := time.Now().Format("2006-01-02 15:04:05")
	action, articleID, err := repository.UpsertArticle(ctx, sources[0].ID, FeedEntry{
		ArticleKey: "article-key-1", ExternalID: "external-1", Title: "测试投资文章",
		Link: "https://example.com/article", Author: "长江证券研究所", PublishedAt: now,
		Summary: "摘要", Content: "正文",
	})
	if err != nil || action != "inserted" || articleID == 0 {
		t.Fatalf("UpsertArticle() = %q, %d, %v", action, articleID, err)
	}
	result := NormalizeAnalysis(AnalysisResult{
		Summary:         "结构化摘要",
		Market:          MarketJudgment{Mood: "optimistic", MoodReason: "风险偏好改善", Prediction: "up", PredictionReason: "流动性改善"},
		Recommendations: []Signal{{Name: "贵州茅台", Type: "stock", Reason: "估值修复"}},
	})
	if err := repository.SaveAnalysis(ctx, articleID, "success", result, "", "deepseek-v4-pro", PromptVersion); err != nil {
		t.Fatalf("SaveAnalysis() error = %v", err)
	}

	// 3. 核对文章列表日期、详情作者和反馈更新。
	articles, err := repository.Articles(ctx, 60, 50)
	if err != nil || len(articles) != 1 {
		t.Fatalf("Articles() = %#v, %v", articles, err)
	}
	if articles[0].Author != "长江证券研究所" || len(articles[0].PublishedAt) != len("2006-01-02") {
		t.Errorf("article = %#v, want full author and date-only value", articles[0])
	}
	detail, err := repository.UpdatePromptFeedback(ctx, articleID, "以后更关注估值")
	if err != nil || detail == nil || detail.Author != "长江证券研究所" || detail.PromptFeedback != "以后更关注估值" {
		t.Errorf("detail = %#v, error = %v", detail, err)
	}

	// 4. 核对 60 天信号和 3 天市场统计。
	service := NewService(repository, ServiceOptions{Model: "deepseek-v4-pro"})
	report, err := service.Report(ctx, 60, 3)
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if len(report.Signals) != 1 || report.Signals[0].Name != pendingSignalGroupName ||
		report.Signals[0].RecommendationCount != 1 ||
		len(report.Signals[0].Members) != 1 || report.Signals[0].Members[0] != "贵州茅台" {
		t.Errorf("signals = %#v", report.Signals)
	}
	if len(report.MoodDistribution) != 1 || report.MoodDistribution[0].Name != "optimistic" {
		t.Errorf("mood distribution = %#v", report.MoodDistribution)
	}
}

// TestSyncDefaultSourcePreservesMigratedSourceWithoutConfig 验证空配置不会禁用已迁移文章来源。
// 输入：一条含真实 URL、启用状态和历史更新时间的现有来源。
// 输出：空配置同步后 URL、状态和更新时间保持不变。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestSyncDefaultSourcePreservesMigratedSourceWithoutConfig(t *testing.T) {
	// 1. 创建迁移库并写入模拟 SQLite 迁移来源。
	ctx := context.Background()
	db := testdatabase.Open(t)
	const feedURL = "http://8.138.123.59:5000/api/rss/all"
	const historical = "2026-07-15 20:02:18"
	if _, err := db.ExecContext(ctx, `INSERT INTO investment_article_source(
		source_code, source_name, source_type, feed_url, weight, is_active, description,
		last_fetch_status, created_at, updated_at
	) VALUES('wechat_aggregate','公众号聚合','wechat_rss_aggregate',?,1,1,'微信公众号聚合 RSS。','success',?,?)`,
		feedURL, historical, historical); err != nil {
		t.Fatalf("insert migrated source: %v", err)
	}

	// 2. 空配置只负责缺失行初始化，不能覆盖持久化来源。
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, ""); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	var storedURL, updatedAt string
	var active int
	if err := db.QueryRowContext(ctx, `SELECT feed_url, is_active, updated_at
		FROM investment_article_source WHERE source_code = 'wechat_aggregate'`).Scan(&storedURL, &active, &updatedAt); err != nil {
		t.Fatalf("query source: %v", err)
	}
	if storedURL != feedURL || active != 1 || updatedAt != historical {
		t.Fatalf("source = url:%q active:%d updated:%q", storedURL, active, updatedAt)
	}
}
