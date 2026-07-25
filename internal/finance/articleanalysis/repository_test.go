package articleanalysis

import (
	"context"
	"fmt"
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
		WithArgs("证券行业", "sector", "deepseek", "test-model").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("INSERT OR IGNORE INTO investment_signal_alias").
		WithArgs(int64(7), "券商", "券商", 0.98, "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec("INSERT OR IGNORE INTO investment_signal_alias").
		WithArgs(int64(7), "中信证券", "中信证券", 0.95, "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(12, 1))
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

// TestSyncDefaultSourceOnlyFillsEmptyFeedURL 验证运行环境地址不会覆盖共享来源地址。
// 输入：当前进程提供一个有效 RSS 地址。
// 输出：重复来源只在原地址为空时补值。
// 副作用：执行一条模拟来源 upsert。
func TestSyncDefaultSourceOnlyFillsEmptyFeedURL(t *testing.T) {
	// 1. 使用查询匹配器拒绝无条件覆盖 feed_url 的 upsert。
	matcher := sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		statement := strings.Join(strings.Fields(actualSQL), " ")
		if !strings.Contains(statement, "feed_url = CASE WHEN investment_article_source.feed_url = '' THEN excluded.feed_url ELSE investment_article_source.feed_url END") {
			return fmt.Errorf("feed_url must only be filled when empty: %s", statement)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectExec("runtime-safe source upsert").
		WithArgs("http://127.0.0.1:15000/api/rss/all", 1, "ready", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 2. 同步默认来源并核对 SQL 满足只补空值约束。
	if err := NewRepository(db).SyncDefaultSource(context.Background(), "http://127.0.0.1:15000/api/rss/all"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
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

// TestRepositoryAndReportPreserveArticleContracts 验证文章存储、详情和 60/3 天报告契约。
// 输入：一篇当前文章和一条结构化分析结果。
// 输出：列表、详情、信号榜和市场分布均返回前端需要的字段。
// 副作用：创建并写入隔离 SQLite 测试 schema。
func TestRepositoryAndReportPreserveArticleContracts(t *testing.T) {
	// 1. 创建完整迁移数据库并同步默认 RSS 来源。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000/rss/all.xml"); err != nil {
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
