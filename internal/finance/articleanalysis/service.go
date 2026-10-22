package articleanalysis

import (
	"context"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
)

type forceArticleFetchContextKey struct{}

const (
	articleAnalysisMaxAttempts  = 2
	articleAnalysisMaxTokens    = 3600
	scheduledFetchLimit         = 1000
	scheduledAnalysisBatchLimit = 50
	scheduledAnalysisMaxBatches = 10
	scheduledSourceStaleAfter   = 72 * time.Hour
	pendingSignalGroupName      = "待归类"
	pendingSignalGroupType      = "pending"
	unclearSignalGroupName      = "信息不明确"
	unclearSignalGroupType      = "other"
)

// ArticleGateway 定义投资文章服务需要的外部文章读取能力。
type ArticleGateway interface {
	Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.ArticleItem, error)
}

// AnalysisGateway 定义投资文章服务需要的模型分析能力。
type AnalysisGateway interface {
	Configured() bool
	SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// AnalysisModelConfig 把一个页面模型选项绑定到对应的模型客户端。
type AnalysisModelConfig struct {
	ID       string
	Provider string
	Model    string
	Label    string
	Analyzer AnalysisGateway
}

type analysisModelRuntime struct {
	AnalysisModelConfig
}

// ServiceOptions 描述投资文章服务的当前进程文章来源和模型配置。
type ServiceOptions struct {
	FeedURL                string
	Articles               ArticleGateway
	WeRead                 *WeReadSource
	AnalysisModels         []AnalysisModelConfig
	DefaultAnalysisModelID string
	Now                    func() time.Time
}

// Service 提供文章页面、任务和手动接口复用的业务入口。
type Service struct {
	repository             *Repository
	options                ServiceOptions
	analysisModels         map[string]analysisModelRuntime
	analysisModelOrder     []string
	defaultAnalysisModelID string
}

// NewService 创建投资文章分析服务。
// 输入：repository 提供 PostgreSQL 访问，options 提供模型名称。
// 输出：返回文章服务。
// 副作用：无。
func NewService(repository *Repository, options ServiceOptions) *Service {
	// 1. 所有调用方统一使用显式模型目录。
	options.FeedURL = strings.TrimSpace(options.FeedURL)
	models := options.AnalysisModels
	runtimes := make(map[string]analysisModelRuntime, len(models))
	order := make([]string, 0, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Model = strings.TrimSpace(model.Model)
		if model.ID == "" || model.Model == "" || model.Analyzer == nil {
			continue
		}
		if model.Label == "" {
			model.Label = model.Model
		}
		if _, exists := runtimes[model.ID]; exists {
			continue
		}
		runtimes[model.ID] = analysisModelRuntime{AnalysisModelConfig: model}
		order = append(order, model.ID)
	}
	defaultID := strings.TrimSpace(options.DefaultAnalysisModelID)
	if _, exists := runtimes[defaultID]; !exists && len(order) > 0 {
		defaultID = order[0]
	}
	return &Service{
		repository: repository, options: options, analysisModels: runtimes,
		analysisModelOrder: order, defaultAnalysisModelID: defaultID,
	}
}
