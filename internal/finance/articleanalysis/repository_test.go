package articleanalysis

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestRepositoryAndReportPreserveArticleContracts 验证文章存储、详情和 60/3 天报告契约。
// 输入：一篇当前文章和一条结构化分析结果。
// 输出：列表、详情、信号榜和市场分布均返回前端需要的字段。
// 副作用：在测试临时目录创建并写入 SQLite 文件。
func TestRepositoryAndReportPreserveArticleContracts(t *testing.T) {
	// 1. 创建完整迁移数据库并同步默认 RSS 来源。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{Path: filepath.Join(t.TempDir(), "articles.db")})
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
	sources, err := repository.Sources(ctx, true)
	if err != nil || len(sources) != 1 {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}

	// 2. 写入文章和成功分析结果，作者保留全名。
	now := time.Now().Format(time.RFC3339)
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
	if len(report.Signals) != 1 || report.Signals[0].Name != "贵州茅台" || report.Signals[0].RecommendationCount != 1 {
		t.Errorf("signals = %#v", report.Signals)
	}
	if len(report.MoodDistribution) != 1 || report.MoodDistribution[0].Name != "optimistic" {
		t.Errorf("mood distribution = %#v", report.MoodDistribution)
	}
}
