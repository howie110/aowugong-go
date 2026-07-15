package articleanalysis

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

type fixedRSSGateway struct {
	pollCount int
}

// Poll 记录文章同步测试中的上游刷新调用。
// 输入：ctx 和 feedURL 模拟正式 RSS 客户端。
// 输出：始终成功。
// 副作用：增加 pollCount。
func (f *fixedRSSGateway) Poll(ctx context.Context, feedURL string) error {
	// 1. 记录调用次数供断言。
	f.pollCount++
	return nil
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
	// 1. 返回 fenced JSON 以验证兼容解析。
	return "```json\n{\"summary\":\"分析摘要\",\"recommendations\":[{\"name\":\"贵州茅台\",\"type\":\"stock\",\"reason\":\"估值有望修复\"}],\"risks\":[],\"market\":{\"mood\":\"cautious\",\"mood_reason\":\"等待确认\",\"prediction\":\"range\",\"prediction_reason\":\"震荡\"}}\n```", nil
}

// TestServiceSyncsRSSAndAnalyzesThroughUnifiedEntry 验证抓取与分析完整业务入口。
// 输入：固定 RSS 和模型客户端、已启用默认来源。
// 输出：同步插入并分析一篇文章，cautious 最终存为 neutral。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestServiceSyncsRSSAndAnalyzesThroughUnifiedEntry(t *testing.T) {
	// 1. 创建数据库、来源和带假客户端的服务。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "article-sync.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
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
	if result.InsertedCount != 1 || result.AnalyzedCount != 1 || rss.pollCount != 1 {
		t.Fatalf("result = %#v, pollCount = %d", result, rss.pollCount)
	}

	// 3. 读取页面列表核对清洗后的结果。
	articles, err := service.Articles(ctx, 60, 50)
	if err != nil || len(articles) != 1 {
		t.Fatalf("Articles() = %#v, %v", articles, err)
	}
	if articles[0].MarketMood != "neutral" || len(articles[0].RecommendationNames) != 1 {
		t.Errorf("article = %#v", articles[0])
	}
}
