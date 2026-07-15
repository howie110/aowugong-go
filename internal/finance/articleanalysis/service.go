package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/howiedata/aowugong-go/internal/client"
)

var fencedJSONPattern = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// RSSGateway 定义投资文章服务需要的 RSS 抓取和 WeChatRSS 刷新能力。
type RSSGateway interface {
	Poll(ctx context.Context, feedURL string) error
	Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.RSSItem, error)
}

// AnalysisGateway 定义投资文章服务需要的模型分析能力。
type AnalysisGateway interface {
	Configured() bool
	SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// ServiceOptions 描述投资文章服务的模型展示配置。
type ServiceOptions struct {
	Model    string
	RSS      RSSGateway
	Analyzer AnalysisGateway
}

// Sync 抓取全部启用来源，并按选项继续分析待处理文章。
// 输入：ctx 控制处理，fetchLimit 是每来源上限，analyze 控制是否分析，analysisLimit 是分析上限。
// 输出：返回来源、抓取、写入和分析统计；基础数据库失败时返回错误。
// 副作用：调用 WeChatRSS/RSS/DeepSeek，并写入 SQLite。
func (s *Service) Sync(ctx context.Context, fetchLimit int, analyze bool, analysisLimit int) (SyncResult, error) {
	// 1. 读取启用来源并初始化稳定空数组结果。
	sources, err := s.repository.sourceRecords(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("读取待同步文章来源: %w", err)
	}
	result := SyncResult{SourceCount: len(sources), FailedSources: []map[string]string{}}
	if fetchLimit < 1 {
		fetchLimit = 30
	}
	if fetchLimit > 100 {
		fetchLimit = 100
	}

	// 2. 逐来源刷新、抓取并 upsert，单来源错误写入统计。
	for _, source := range sources {
		if s.options.RSS == nil {
			message := "RSS 客户端未配置"
			_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
			continue
		}
		if source.SourceType == "wechat_rss_aggregate" {
			if err := s.options.RSS.Poll(ctx, source.FeedURL); err != nil {
				message := err.Error()
				_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
				result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
				continue
			}
		}
		items, err := s.options.RSS.Fetch(ctx, source.ID, source.FeedURL, fetchLimit)
		if err != nil {
			message := err.Error()
			_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
			continue
		}
		inserted, updated := 0, 0
		for _, item := range items {
			action, _, err := s.repository.UpsertArticle(ctx, source.ID, feedEntryFromClient(item))
			if err != nil {
				return SyncResult{}, fmt.Errorf("保存来源 %s 文章: %w", source.SourceName, err)
			}
			if action == "inserted" {
				inserted++
			} else {
				updated++
			}
		}
		message := fmt.Sprintf("fetched=%d, inserted=%d, updated=%d", len(items), inserted, updated)
		if err := s.repository.UpdateSourceStatus(ctx, source.ID, "success", message); err != nil {
			return SyncResult{}, err
		}
		result.FetchedCount += len(items)
		result.InsertedCount += inserted
		result.UpdatedCount += updated
	}

	// 3. 可选分析阶段复用公开批量入口。
	if analyze {
		analysisResult, err := s.AnalyzePending(ctx, analysisLimit)
		if err != nil {
			return SyncResult{}, err
		}
		result.AnalyzedCount = analysisResult.AnalyzedCount
		result.SkippedCount = analysisResult.SkippedCount
		result.ErrorCount = analysisResult.ErrorCount
	}
	return result, nil
}

// AnalyzePending 调用模型分析最近待处理文章。
// 输入：ctx 控制处理，limit 是 1 到 50 的文章上限。
// 输出：返回成功、跳过、错误及逐篇结果；数据库失败时返回错误。
// 副作用：调用 DeepSeek 并写入 SQLite 分析表。
func (s *Service) AnalyzePending(ctx context.Context, limit int) (AnalysisBatchResult, error) {
	// 1. 读取待分析文章并准备非 nil 结果数组。
	articles, err := s.repository.pendingArticles(ctx, limit)
	if err != nil {
		return AnalysisBatchResult{}, fmt.Errorf("读取待分析文章: %w", err)
	}
	result := AnalysisBatchResult{Items: []map[string]any{}}

	// 2. 每篇独立分析和落库，模型错误不阻断下一篇。
	for _, article := range articles {
		item, status, err := s.analyzeOne(ctx, article)
		if err != nil {
			return AnalysisBatchResult{}, err
		}
		result.Items = append(result.Items, item)
		switch status {
		case "success":
			result.AnalyzedCount++
		case "skipped":
			result.SkippedCount++
		default:
			result.ErrorCount++
		}
	}
	return result, nil
}

// analyzeOne 分析单篇文章并持久化最终状态。
// 输入：ctx 控制模型请求，article 是待分析文章。
// 输出：返回页面结果项、业务状态和仅数据库失败时使用的错误。
// 副作用：调用 DeepSeek 并写入 SQLite。
func (s *Service) analyzeOne(ctx context.Context, article pendingArticle) (map[string]any, string, error) {
	// 1. 未配置模型时保留 pending，便于配置后重试。
	if s.options.Analyzer == nil || !s.options.Analyzer.Configured() {
		message := "未配置 DEEPSEEK_API_KEY"
		if err := s.repository.SaveAnalysis(ctx, article.ID, "pending", AnalysisResult{}, message, s.options.Model, PromptVersion); err != nil {
			return nil, "", err
		}
		return map[string]any{"article_id": article.ID, "status": "skipped", "message": message}, "skipped", nil
	}

	// 2. 调用模型并解析、规范化严格 JSON。
	content, err := s.options.Analyzer.SimpleChat(ctx, buildAnalysisPrompt(article), 1600)
	if err != nil {
		message := err.Error()
		if saveErr := s.repository.SaveAnalysis(ctx, article.ID, "error", AnalysisResult{}, message, s.options.Model, PromptVersion); saveErr != nil {
			return nil, "", saveErr
		}
		return map[string]any{"article_id": article.ID, "status": "error", "message": message}, "error", nil
	}
	parsed, err := parseAnalysisJSON(content)
	if err != nil {
		message := "DeepSeek JSON 解析失败：" + err.Error()
		if saveErr := s.repository.SaveAnalysis(ctx, article.ID, "error", AnalysisResult{}, message, s.options.Model, PromptVersion); saveErr != nil {
			return nil, "", saveErr
		}
		return map[string]any{"article_id": article.ID, "status": "error", "message": message}, "error", nil
	}
	normalized := NormalizeAnalysis(parsed)
	if err := s.repository.SaveAnalysis(ctx, article.ID, "success", normalized, "", s.options.Model, PromptVersion); err != nil {
		return nil, "", err
	}
	return map[string]any{"article_id": article.ID, "status": "success"}, "success", nil
}

// parseAnalysisJSON 解码模型纯 JSON 或 Markdown fenced JSON。
// 输入：content 是模型文本。
// 输出：返回结构化结果；JSON 无效时返回错误。
// 副作用：无。
func parseAnalysisJSON(content string) (AnalysisResult, error) {
	// 1. 优先提取 fenced 代码块并清理空白。
	content = strings.TrimSpace(content)
	if matches := fencedJSONPattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}

	// 2. 使用标准 JSON 解码器读取固定模型。
	var result AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return AnalysisResult{}, err
	}
	return result, nil
}

// feedEntryFromClient 把通用 RSS 客户端模型转换为文章仓储模型。
// 输入：item 是客户端规范化文章。
// 输出：返回 repository 使用的 FeedEntry。
// 副作用：无。
func feedEntryFromClient(item client.RSSItem) FeedEntry {
	// 1. 一一映射字段，业务包继续拥有存储模型。
	return FeedEntry{
		ArticleKey: item.ArticleKey, ExternalID: item.ExternalID, Title: item.Title,
		Link: item.Link, Author: item.Author, PublishedAt: item.PublishedAt,
		Summary: item.Summary, Content: item.Content, RawEntry: item.RawEntry,
	}
}

// Service 提供文章页面、任务和手动接口复用的业务入口。
type Service struct {
	repository *Repository
	options    ServiceOptions
}

// NewService 创建投资文章分析服务。
// 输入：repository 提供 SQLite 访问，options 提供模型名称。
// 输出：返回文章服务。
// 副作用：无。
func NewService(repository *Repository, options ServiceOptions) *Service {
	// 1. 应用当前模型默认值并保存依赖。
	if options.Model == "" {
		options.Model = "deepseek-v4-pro"
	}
	return &Service{repository: repository, options: options}
}

// AnalysisSummary 构建投资文章分析页面摘要。
// 输入：ctx 控制 SQLite 查询。
// 输出：返回文章和已分析计数；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) AnalysisSummary(ctx context.Context) (PageSummary, error) {
	// 1. 读取统一计数口径。
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return PageSummary{}, fmt.Errorf("读取文章分析摘要: %w", err)
	}

	// 2. 组装前端状态卡片。
	return PageSummary{
		Title:       "投资文章分析",
		Description: "统计最近资讯里的标的信号、市场氛围和涨跌预测。",
		Metrics: []PageMetric{
			{Label: "文章", Value: strconv.Itoa(counts.ArticleCount), Detail: "已入库文章", Status: "normal"},
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "DeepSeek 结构化结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// FetchSummary 构建投资文章抓取页面摘要。
// 输入：ctx 控制 SQLite 查询。
// 输出：返回来源、文章、待分析和已分析计数；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) FetchSummary(ctx context.Context) (PageSummary, error) {
	// 1. 读取统一计数口径。
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return PageSummary{}, fmt.Errorf("读取文章抓取摘要: %w", err)
	}

	// 2. 组装抓取页状态卡片。
	return PageSummary{
		Title:       "投资文章抓取",
		Description: "管理信息源，抓取 RSS 文章，并触发 DeepSeek 结构化分析。",
		Metrics: []PageMetric{
			{Label: "来源", Value: strconv.Itoa(counts.SourceCount), Detail: "启用的信息源", Status: "normal"},
			{Label: "文章", Value: strconv.Itoa(counts.ArticleCount), Detail: "已入库文章", Status: "normal"},
			{Label: "待分析", Value: strconv.Itoa(counts.PendingCount), Detail: "抓取后等待模型处理", Status: "normal"},
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "DeepSeek 结构化结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// Sources 返回页面展示的信息源列表。
// 输入：ctx 控制 SQLite 查询。
// 输出：返回全部来源；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Sources(ctx context.Context) ([]Source, error) {
	// 1. 页面需要同时看到未配置来源状态。
	return s.repository.Sources(ctx, false)
}

// Articles 返回指定天数内已分析文章。
// 输入：ctx 控制查询，days 和 limit 限制范围。
// 输出：返回文章列表；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Articles(ctx context.Context, days, limit int) ([]ArticleItem, error) {
	// 1. 复用仓储层受限查询。
	return s.repository.Articles(ctx, days, limit)
}

// Detail 返回单篇文章详情。
// 输入：ctx 控制查询，articleID 是文章主键。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Detail(ctx context.Context, articleID int64) (*ArticleDetail, error) {
	// 1. 复用仓储层详情映射。
	return s.repository.Detail(ctx, articleID)
}

// UpdatePromptFeedback 保存管理员修正意见并返回最新详情。
// 输入：ctx 控制写入，articleID 是文章主键，feedback 是修正意见。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：写入 SQLite。
func (s *Service) UpdatePromptFeedback(ctx context.Context, articleID int64, feedback string) (*ArticleDetail, error) {
	// 1. 由仓储层统一截断并更新反馈。
	return s.repository.UpdatePromptFeedback(ctx, articleID, feedback)
}

// Report 构建信号榜和短期市场分布。
// 输入：ctx 控制查询，targetDays 默认 60，marketDays 默认 3。
// 输出：返回完整分析报告；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Report(ctx context.Context, targetDays, marketDays int) (Report, error) {
	// 1. 分别读取信号榜和市场判断的独立日期范围。
	if targetDays < 1 {
		targetDays = DefaultTargetDays
	}
	if marketDays < 1 {
		marketDays = DefaultMarketDays
	}
	targetRows, err := s.repository.analysisRows(ctx, targetDays)
	if err != nil {
		return Report{}, fmt.Errorf("读取信号榜统计: %w", err)
	}
	marketRows, err := s.repository.analysisRows(ctx, marketDays)
	if err != nil {
		return Report{}, fmt.Errorf("读取短期市场统计: %w", err)
	}

	// 2. 聚合推荐、风险及市场枚举并附带模型说明。
	return Report{
		AnalysisModel:          s.options.Model,
		AnalysisPrompt:         AnalysisPromptTemplate(),
		PromptVersion:          PromptVersion,
		Signals:                buildSignalStats(targetRows),
		MoodDistribution:       buildDistribution(marketRows, true),
		PredictionDistribution: buildDistribution(marketRows, false),
	}, nil
}

type signalAccumulator struct {
	SignalStat
	LatestAt string
}

// buildSignalStats 合并推荐和风险为每个标的一行。
// 输入：rows 是目标天数内分析行。
// 输出：按总次数和最近出现时间倒序返回信号榜。
// 副作用：无。
func buildSignalStats(rows []analysisRow) []SignalStat {
	// 1. 按标的名称累计两侧次数和最近日期。
	grouped := make(map[string]*signalAccumulator)
	merge := func(signals []Signal, recommendation bool, occurredAt string) {
		for _, signal := range signals {
			if signal.Name == "" {
				continue
			}
			item, exists := grouped[signal.Name]
			if !exists {
				item = &signalAccumulator{SignalStat: SignalStat{Name: signal.Name, Type: signal.Type}, LatestAt: occurredAt}
				if item.Type == "" {
					item.Type = "other"
				}
				grouped[signal.Name] = item
			}
			if item.Type == "other" && signal.Type != "" && signal.Type != "other" {
				item.Type = signal.Type
			}
			if recommendation {
				item.RecommendationCount++
			} else {
				item.RiskCount++
			}
			item.Count++
			if occurredAt > item.LatestAt {
				item.LatestAt = occurredAt
			}
		}
	}
	for _, row := range rows {
		merge(row.Recommendations, true, row.OccurredAt)
		merge(row.Risks, false, row.OccurredAt)
	}

	// 2. 排序并移除内部最近日期字段。
	items := make([]*signalAccumulator, 0, len(grouped))
	for _, item := range grouped {
		items = append(items, item)
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].Count == items[right].Count {
			return items[left].LatestAt > items[right].LatestAt
		}
		return items[left].Count > items[right].Count
	})
	results := make([]SignalStat, 0, len(items))
	for _, item := range items {
		results = append(results, item.SignalStat)
	}
	return results
}

// buildDistribution 统计市场氛围或涨跌预测分布。
// 输入：rows 是市场天数内分析行，mood 控制统计字段。
// 输出：按次数倒序、名称正序返回分布。
// 副作用：无。
func buildDistribution(rows []analysisRow, mood bool) []DistributionItem {
	// 1. 规范化枚举并累计次数。
	counts := make(map[string]int)
	for _, row := range rows {
		key := normalizePrediction(row.Prediction)
		if mood {
			key = normalizeMood(row.MarketMood)
		}
		counts[key]++
	}
	results := make([]DistributionItem, 0, len(counts))
	for name, count := range counts {
		results = append(results, DistributionItem{Name: name, Count: count})
	}

	// 2. 保持高频项优先并稳定同次数顺序。
	sort.Slice(results, func(left, right int) bool {
		if results[left].Count == results[right].Count {
			return results[left].Name < results[right].Name
		}
		return results[left].Count > results[right].Count
	})
	return results
}

// AnalysisPromptTemplate 返回页面展示的当前提示词模板。
// 输入：无。
// 输出：返回带占位符的中文模板。
// 副作用：无。
func AnalysisPromptTemplate() string {
	// 1. 提示词只维护一份，页面展示和模型请求共同复用。
	return buildAnalysisPrompt(pendingArticle{
		SourceName: "{来源名称}", SourceType: "{来源类型}", Title: "{文章标题}",
		Link: "{原文链接}", PublishedAt: "{发布时间}", Content: "{文章正文，最多取前 12000 字}",
	})
}

// buildAnalysisPrompt 根据文章内容生成 DeepSeek 结构化提示词。
// 输入：article 是待分析文章。
// 输出：返回严格 JSON 输出约束提示词。
// 副作用：无。
func buildAnalysisPrompt(article pendingArticle) string {
	// 1. 正文为空时回退摘要并限制最大字符数。
	content := article.Content
	if content == "" {
		content = article.Summary
	}
	content = truncateRunes(content, 12000)

	// 2. 返回与当前 prompt 版本一致的完整模板。
	return fmt.Sprintf(`你是一个投资资讯结构化分析助手。请提取对未来投资判断有指导意义的结构化信息。
只基于文章内容，不要编造没有提到的标的或结论。
请严格只返回 JSON，不要 Markdown，不要解释。无法判断时使用 "unknown" 或空数组。

抽取规则：
1. market 只表示短期判断，通常对应未来数日到数周的市场氛围和涨跌预测；mood 和 prediction 都必须给出简短 reason。
2. recommendations / risks 不区分周期；文章里的短期、中期、长期逻辑都可以进入同一个标的信号列表。
3. 一篇文章里的同一个标的只能有一个最终结果：偏正面放 recommendations，偏负面放 risks；绝不能同一个 name 同时出现在 recommendations 和 risks。
4. 不要把同一个标的拆成多条。若文章详细分析一个标的的优点和缺点，请先综合权衡，最后只输出该标的一条最终结论。
5. reason 要写清楚最终判断的核心依据，可以同时概括主要优点和主要风险，但必须服务于最终的推荐/风险结论。
6. 标的 name 必须精简，适合网页表格展示；只保留核心可投资标的，不要句子、不要长描述、不要符号堆叠。
7. 剔除纯结果导向的信息：例如已经发生的涨跌、排名、收益结果。
8. 只有当文章给出面向未来的理由时才抽取标的，例如估值、盈利、政策、供需、周期、库存、订单、流动性、风险事件、配置价值、催化或基本面变化。
9. 如果一句话只是描述当前涨跌、过去收益、资金当下流向，而没有未来判断，请不要抽取为 recommendations 或 risks。

JSON 结构：
{"summary":"80字以内中文摘要","recommendations":[{"name":"标的名称","type":"stock|sector|index|commodity|crypto|concept|other","reason":"80字以内综合原因"}],"risks":[],"market":{"mood":"very_optimistic|optimistic|neutral|pessimistic|very_pessimistic|unknown","mood_reason":"80字以内原因","prediction":"up|down|range|unknown","prediction_reason":"80字以内原因"}}

来源：%s
来源类型：%s
标题：%s
链接：%s
发布时间：%s

文章内容：
%s`, article.SourceName, article.SourceType, article.Title, article.Link, article.PublishedAt, content)
}
