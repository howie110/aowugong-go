package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
)

// TestArticleAnalysisArticlesAcceptsFullSignalWindowLimit 验证文章接口允许页面读取完整六十天窗口。
// 输入：携带 limit=5000 的文章列表请求和空查询结果。
// 输出：返回 200，仓储收到五千条上限。
// 副作用：执行模拟 SQLite 查询并写入测试 HTTP 响应。
func TestArticleAnalysisArticlesAcceptsFullSignalWindowLimit(t *testing.T) {
	// 1. 准备期望五千条上限的模拟文章查询。
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

	// 2. 直接执行已由路由完成鉴权的处理器并核对响应和数据库参数。
	service := articleanalysis.NewService(articleanalysis.NewRepository(db), articleanalysis.ServiceOptions{})
	handler := articleAnalysisHandlers{service: service}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/finance/article-analysis/articles?days=60&limit=5000", nil)
	recorder := httptest.NewRecorder()
	handler.articles(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}
