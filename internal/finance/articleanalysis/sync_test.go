package articleanalysis

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type fixedRSSGateway struct{}

type manyRSSGateway struct {
	count int
}

type recordingRSSGateway struct {
	feedURL string
}

// Fetch 记录当前进程实际使用的 RSS 地址。
// 输入：ctx、sourceID、feedURL 和 limit 模拟正式客户端。
// 输出：返回空文章集合。
// 副作用：把 feedURL 保存到测试替身。
func (g *recordingRSSGateway) Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.RSSItem, error) {
	// 1. 记录地址并返回稳定空集合。
	g.feedURL = feedURL
	return []client.RSSItem{}, nil
}

// TestServiceUsesRuntimeFeedURL 验证共享数据库地址不会覆盖当前进程配置。
// 输入：数据库保存服务器地址，当前进程配置本地 SSH 隧道地址。
// 输出：RSS 客户端收到当前进程配置地址。
// 副作用：执行模拟来源查询和状态更新。
func TestServiceUsesRuntimeFeedURL(t *testing.T) {
	// 1. 创建包含服务器持久化地址的模拟文章来源。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT id, source_code, source_name, source_type, feed_url, is_active").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_code", "source_name", "source_type", "feed_url", "is_active",
			"description", "last_fetch_at", "last_fetch_status", "last_fetch_message",
		}).AddRow(1, "wechat_aggregate", "公众号聚合", "wechat_rss_aggregate",
			"http://127.0.0.1:5000/api/rss/all", 1, "", "", "success", ""))
	mock.ExpectExec("UPDATE investment_article_source").WillReturnResult(sqlmock.NewResult(0, 1))

	// 2. 执行本地同步并核对只使用本地进程的隧道地址。
	rss := &recordingRSSGateway{}
	service := NewService(NewRepository(db), ServiceOptions{
		RSS: rss, FeedURL: "http://127.0.0.1:15000/api/rss/all",
	})
	if _, err := service.Sync(context.Background(), 30, false, 0); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if rss.feedURL != "http://127.0.0.1:15000/api/rss/all" {
		t.Fatalf("Fetch() feedURL = %q", rss.feedURL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// Fetch 返回跨越单批分析上限的文章集合。
// 输入：ctx、sourceID、feedURL 和 limit 模拟正式客户端。
// 输出：返回 count 篇具有唯一键的文章。
// 副作用：无。
func (g manyRSSGateway) Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.RSSItem, error) {
	// 1. 按配置数量生成稳定文章，验证任务会继续下一分析批次。
	items := make([]client.RSSItem, 0, g.count)
	for index := 0; index < g.count; index++ {
		items = append(items, client.RSSItem{
			ArticleKey: fmt.Sprintf("scheduled-%03d", index), ExternalID: fmt.Sprintf("scheduled-%03d", index),
			Title: fmt.Sprintf("定时文章 %03d", index), Link: fmt.Sprintf("https://example.com/scheduled/%d", index),
			Author: "完整作者", PublishedAt: shanghaiNowText(), Summary: "摘要", Content: "正文",
		})
	}
	return items, nil
}

// Fetch 返回文章同步测试使用的一篇固定文章。
// 输入：ctx、sourceID、feedURL 和 limit 模拟正式客户端。
// 输出：返回一篇当前文章。
// 副作用：无。
func (f *fixedRSSGateway) Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.RSSItem, error) {
	// 1. 返回稳定键和完整作者的规范化文章。
	return []client.RSSItem{{
		ArticleKey: "sync-article-key", ExternalID: "sync-1", Title: "同步文章",
		Link: "https://example.com/sync", Author: "完整作者", PublishedAt: shanghaiNowText(),
		Summary: "同步摘要", Content: "同步正文",
	}}, nil
}

type fixedAnalysisGateway struct{}

// Configured 表示测试模型客户端已配置。
// 输入：无。
// 输出：返回 true。
// 副作用：无。
func (fixedAnalysisGateway) Configured() bool {
	// 1. 允许同步流程进入分析阶段。
	return true
}

// SimpleChat 返回测试使用的结构化模型 JSON。
// 输入：ctx、prompt 和 maxTokens 模拟正式模型客户端。
// 输出：返回 cautious 市场氛围和一个推荐信号。
// 副作用：无。
func (fixedAnalysisGateway) SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// 1. 分类提示词返回规范概念组，普通提示词返回文章分析结果。
	if strings.Contains(prompt, "本轮待分类名称") {
		return `{"groups":[{"canonical_name":"白酒行业","type":"sector","aliases":[{"name":"贵州茅台","confidence":0.96}]}]}`, nil
	}
	return "```json\n{\"summary\":\"分析摘要\",\"recommendations\":[{\"name\":\"贵州茅台\",\"type\":\"stock\",\"reason\":\"估值有望修复\"}],\"risks\":[],\"market\":{\"mood\":\"cautious\",\"mood_reason\":\"等待确认\",\"prediction\":\"range\",\"prediction_reason\":\"震荡\"}}\n```", nil
}

// TestServiceSyncsRSSAndAnalyzesThroughUnifiedEntry 验证抓取与分析完整业务入口。
// 输入：固定 RSS 和模型客户端、已启用默认来源。
// 输出：同步插入并分析一篇文章，cautious 最终存为 neutral。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestServiceSyncsRSSAndAnalyzesThroughUnifiedEntry(t *testing.T) {
	// 1. 创建数据库、来源和带假客户端的服务。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000/rss/all.xml"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	rss := &fixedRSSGateway{}
	service := NewService(repository, ServiceOptions{Model: "test-model", RSS: rss, Analyzer: fixedAnalysisGateway{}})

	// 2. 通过统一入口执行抓取和分析。
	result, err := service.Sync(ctx, 30, true, 10)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.InsertedCount != 1 || result.UpdatedCount != 0 || result.AnalyzedCount != 1 {
		t.Fatalf("first result = %#v", result)
	}
	if result.ClassifiedAliasCount != 0 {
		t.Fatalf("direct sync classified aliases = %d, want 0", result.ClassifiedAliasCount)
	}

	// 3. 再次读取同一份 WeChatRSS 数据时跳过已有文章，不重复更新大字段。
	repeated, err := service.Sync(ctx, 30, false, 0)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if repeated.InsertedCount != 0 || repeated.UpdatedCount != 0 {
		t.Fatalf("second result = %#v", repeated)
	}

	// 4. 读取页面列表核对清洗后的结果。
	articles, err := service.Articles(ctx, 60, 50)
	if err != nil || len(articles) != 1 {
		t.Fatalf("Articles() = %#v, %v", articles, err)
	}
	if articles[0].MarketMood != "neutral" || len(articles[0].RecommendationNames) != 1 {
		t.Errorf("article = %#v", articles[0])
	}
}

// TestServiceScheduledSyncDrainsMultipleAnalysisBatches 验证生产任务会清空多批待分析文章。
// 输入：一次抓取五十一篇文章，单批分析上限为五十。
// 输出：返回五十一篇成功、零待处理，证明不是只执行一批。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestServiceScheduledSyncDrainsMultipleAnalysisBatches(t *testing.T) {
	// 1. 创建包含默认来源的临时数据库和批量文章服务。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000/rss/all.xml"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	service := NewService(repository, ServiceOptions{
		Model: "test-model", RSS: manyRSSGateway{count: 51}, Analyzer: fixedAnalysisGateway{},
	})

	// 2. 执行生产同步入口并核对跨批累计和最终 pending。
	result, err := service.SyncScheduled(ctx, true)
	if err != nil {
		t.Fatalf("SyncScheduled() error = %v", err)
	}
	if result.FetchedCount != 51 || result.AnalyzedCount != 51 || result.PendingCount != 0 || result.ClassifiedAliasCount != 1 {
		t.Errorf("result = %#v", result)
	}
}

// TestServiceScheduledSyncFailsWithPendingArticles 验证模型未配置时任务必须失败告警。
// 输入：抓取一篇文章且不配置 DeepSeek 客户端。
// 输出：返回 pending=1 和包含未配置密钥的错误。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestServiceScheduledSyncFailsWithPendingArticles(t *testing.T) {
	// 1. 创建无分析客户端的临时文章服务。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultSource(ctx, "http://127.0.0.1:5000/rss/all.xml"); err != nil {
		t.Fatalf("SyncDefaultSource() error = %v", err)
	}
	service := NewService(repository, ServiceOptions{RSS: manyRSSGateway{count: 1}})

	// 2. 执行生产入口并要求 pending 不会被静默吞掉。
	result, err := service.SyncScheduled(ctx, true)
	if err == nil || !strings.Contains(err.Error(), "未配置 DEEPSEEK_API_KEY") {
		t.Fatalf("SyncScheduled() error = %v, result = %#v", err, result)
	}
	if result.PendingCount != 1 {
		t.Errorf("pending = %d, want 1", result.PendingCount)
	}
}

// TestServiceScheduledSyncReportsMissingClassifierForUnknownSignals 验证自动任务不会静默跳过历史信号归类。
// 输入：没有待分析文章、存在未知信号且未配置 DeepSeek 的自动同步。
// 输出：返回明确的密钥缺失错误。
// 副作用：执行模拟 MySQL 来源、计数和统计查询。
func TestServiceScheduledSyncReportsMissingClassifierForUnknownSignals(t *testing.T) {
	// 1. 准备空来源、零待分析文章和一个未映射信号。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT id, source_code, source_name, source_type, feed_url, is_active").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_code", "source_name", "source_type", "feed_url", "is_active",
			"description", "last_fetch_at", "last_fetch_status", "last_fetch_message",
		}))
	mock.ExpectQuery("SELECT[[:space:]]+\\(SELECT COUNT\\(\\*\\) FROM investment_article_source").
		WillReturnRows(sqlmock.NewRows([]string{"sources", "articles", "analyzed", "pending", "latest"}).
			AddRow(0, 1, 1, 0, "2026-07-20 10:00:00"))
	mock.ExpectQuery("SELECT COALESCE\\(analysis.recommendations_json").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recommendations_json", "risks_json", "market_mood", "prediction", "occurred_at"}).
			AddRow(`[{"name":"券商","type":"sector"}]`, `[]`, "neutral", "range", "2026-07-20 10:00:00"))
	mock.ExpectQuery("SELECT g.id, g.canonical_name, g.group_type, a.alias_name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "canonical_name", "group_type", "alias_name"}))

	// 2. 执行自动完整同步并要求未知信号触发明确配置错误。
	service := NewService(NewRepository(db), ServiceOptions{})
	_, err = service.SyncScheduled(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "未配置 DEEPSEEK_API_KEY") {
		t.Fatalf("SyncScheduled() error = %v, want missing classifier error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}
