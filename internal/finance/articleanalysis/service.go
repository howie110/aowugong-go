package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
)

var fencedJSONPattern = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

const (
	scheduledFetchLimit         = 2000
	scheduledAnalysisBatchLimit = 50
	scheduledAnalysisMaxBatches = 10
	scheduledRSSStaleAfter      = 72 * time.Hour
	pendingSignalGroupName      = "待归类"
	pendingSignalGroupType      = "pending"
)

// RSSGateway 定义投资文章服务需要的 RSS 读取能力。
type RSSGateway interface {
	Fetch(ctx context.Context, sourceID int64, feedURL string, limit int) ([]client.RSSItem, error)
}

// AnalysisGateway 定义投资文章服务需要的模型分析能力。
type AnalysisGateway interface {
	Configured() bool
	SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// ServiceOptions 描述投资文章服务的当前进程 RSS 和模型配置。
type ServiceOptions struct {
	Model    string
	FeedURL  string
	RSS      RSSGateway
	Analyzer AnalysisGateway
	Now      func() time.Time
}

// Sync 抓取全部启用来源，并按选项继续分析待处理文章。
// 输入：ctx 控制处理，fetchLimit 是每来源上限，analyze 控制是否分析，analysisLimit 是分析上限。
// 输出：返回来源、抓取、写入和分析统计；基础数据库失败时返回错误。
// 副作用：读取 WeChatRSS/RSS，按需调用 DeepSeek，并写入 SQLite。
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
	if fetchLimit > scheduledFetchLimit {
		fetchLimit = scheduledFetchLimit
	}

	// 2. 逐来源读取现有 RSS 数据并写入新文章，单来源错误写入统计。
	for _, source := range sources {
		if s.options.RSS == nil {
			message := "RSS 客户端未配置"
			_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
			continue
		}
		feedURL := source.FeedURL
		if source.SourceType == "wechat_rss_aggregate" && s.options.FeedURL != "" {
			feedURL = s.options.FeedURL
		}
		items, err := s.options.RSS.Fetch(ctx, source.ID, feedURL, fetchLimit)
		if err != nil {
			message := err.Error()
			_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
			continue
		}
		inserted, updated, unchanged := 0, 0, 0
		for _, item := range items {
			if item.PublishedAt > result.LatestFetchedAt {
				result.LatestFetchedAt = item.PublishedAt
			}
			action, _, err := s.repository.UpsertArticle(ctx, source.ID, feedEntryFromClient(item))
			if err != nil {
				return SyncResult{}, fmt.Errorf("保存来源 %s 文章: %w", source.SourceName, err)
			}
			switch action {
			case "inserted":
				inserted++
			case "updated":
				updated++
			default:
				unchanged++
			}
		}
		message := fmt.Sprintf("fetched=%d, inserted=%d, updated=%d, unchanged=%d", len(items), inserted, updated, unchanged)
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

// SyncScheduled 执行生产任务使用的完整抓取和分批分析流程。
// 输入：ctx 控制处理，classifySignals 控制是否补齐六十天信号概念映射。
// 输出：返回累计同步统计；来源失败、模型缺失或仍有待分析文章时返回错误。
// 副作用：调用 WeChatRSS、RSS、DeepSeek，并写入 SQLite。
func (s *Service) SyncScheduled(ctx context.Context, classifySignals bool) (SyncResult, error) {
	// 1. 抓取全部来源的当前文章，来源失败时保留明细并立即升级为任务错误。
	result, err := s.Sync(ctx, scheduledFetchLimit, false, 0)
	if err != nil {
		return result, fmt.Errorf("抓取投资文章: %w", err)
	}
	if len(result.FailedSources) > 0 {
		return result, fmt.Errorf("投资文章抓取存在失败来源: %s", formatFailedSources(result.FailedSources))
	}

	// 2. 读取抓取后 pending；模型未配置时保留数据并明确失败告警。
	if err := s.validateScheduledRSSFreshness(result); err != nil {
		return result, err
	}
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return result, fmt.Errorf("读取文章分析进度: %w", err)
	}
	result.PendingCount = counts.PendingCount
	if result.PendingCount > 0 && (s.options.Analyzer == nil || !s.options.Analyzer.Configured()) {
		return result, fmt.Errorf("未配置 DEEPSEEK_API_KEY，仍有 %d 篇投资文章等待分析", result.PendingCount)
	}

	// 3. 最多执行十个五十篇批次，持续累计并在没有成功进展时停止重试。
	for batch := 0; batch < scheduledAnalysisMaxBatches && result.PendingCount > 0; batch++ {
		analysisResult, analyzeErr := s.AnalyzePending(ctx, scheduledAnalysisBatchLimit)
		if analyzeErr != nil {
			return result, fmt.Errorf("分析第 %d 批投资文章: %w", batch+1, analyzeErr)
		}
		result.AnalyzedCount += analysisResult.AnalyzedCount
		result.SkippedCount += analysisResult.SkippedCount
		result.ErrorCount += analysisResult.ErrorCount
		counts, err = s.repository.counts(ctx)
		if err != nil {
			return result, fmt.Errorf("读取第 %d 批文章分析进度: %w", batch+1, err)
		}
		result.PendingCount = counts.PendingCount
		if analysisResult.AnalyzedCount == 0 {
			break
		}
	}

	// 4. pending、跳过或错误都视为未完整完成，交给任务包装器发送失败通知。
	if result.PendingCount > 0 || result.SkippedCount > 0 || result.ErrorCount > 0 {
		return result, fmt.Errorf("投资文章分析未正常完成: 待分析=%d, 已分析=%d, 已跳过=%d, 错误=%d",
			result.PendingCount, result.AnalyzedCount, result.SkippedCount, result.ErrorCount)
	}

	// 5. 自动或 CLI 任务补扫六十天未知名称；页面手动任务跳过，避免额外等待。
	if !classifySignals {
		return result, nil
	}
	classifiedCount, err := s.classifySignalAliases(ctx, DefaultTargetDays)
	if err != nil {
		return result, fmt.Errorf("归类投资信号: %w", err)
	}
	result.ClassifiedAliasCount += classifiedCount
	return result, nil
}

// validateScheduledRSSFreshness 检查定时抓取的上游 RSS 是否长期没有新文章。
// 输入：result 是本次 RSS 抓取统计，包含上游最新发布时间。
// 输出：上游最新文章超过固定阈值时返回错误；正常或没有可判断时间时返回 nil。
// 副作用：无，不访问数据库、不发送通知。
func (s *Service) validateScheduledRSSFreshness(result SyncResult) error {
	// 1. 只有 RSS 成功返回文章并带发布时间时，才判断上游是否停更。
	if result.FetchedCount == 0 || strings.TrimSpace(result.LatestFetchedAt) == "" {
		return nil
	}
	latest, err := time.Parse("2006-01-02 15:04:05", result.LatestFetchedAt)
	if err != nil {
		return nil
	}

	// 2. 使用可注入时钟计算滞后时间，超过三天交给任务包装器失败通知。
	now := time.Now
	if s.options.Now != nil {
		now = s.options.Now
	}
	lag := now().UTC().Sub(latest.UTC())
	if lag > scheduledRSSStaleAfter {
		return fmt.Errorf("WeChatRSS 上游最新文章过旧: 最新=%s, 已滞后=%s, 请检查微信登录或公众号抓取", result.LatestFetchedAt, formatDurationHours(lag))
	}
	return nil
}

// formatDurationHours 把持续时间压缩成通知可读的小时文本。
// 输入：duration 是需要展示的持续时间。
// 输出：返回形如 192h 的整数小时文本。
// 副作用：无。
func formatDurationHours(duration time.Duration) string {
	// 1. 使用向下取整小时，避免通知里出现过长的小数秒。
	if duration < time.Hour {
		return duration.String()
	}
	return fmt.Sprintf("%dh", int(duration.Hours()))
}

// formatFailedSources 把失败文章来源和原因合并为通知可读文本。
// 输入：sources 是同步结果中的失败来源映射。
// 输出：返回使用分号分隔的来源与错误文本。
// 副作用：无。
func formatFailedSources(sources []map[string]string) string {
	// 1. 为缺失字段提供稳定名称并按原抓取顺序连接。
	details := make([]string, 0, len(sources))
	for _, source := range sources {
		name, message := strings.TrimSpace(source["source"]), strings.TrimSpace(source["error"])
		if name == "" {
			name = "未知来源"
		}
		if message == "" {
			message = "未知错误"
		}
		details = append(details, name+":"+message)
	}
	return strings.Join(details, "; ")
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
	options.FeedURL = strings.TrimSpace(options.FeedURL)
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
	groups, err := s.repository.SignalGroups(ctx)
	if err != nil {
		return Report{}, err
	}

	// 2. 聚合推荐、风险及市场枚举并附带模型说明。
	return Report{
		AnalysisModel:          s.options.Model,
		AnalysisPrompt:         AnalysisPromptTemplate(),
		PromptVersion:          PromptVersion,
		Signals:                buildSignalStats(targetRows, groups),
		MoodDistribution:       buildDistribution(marketRows, true),
		PredictionDistribution: buildDistribution(marketRows, false),
	}, nil
}

type signalAccumulator struct {
	SignalStat
	LatestAt   string
	memberKeys map[string]struct{}
}

type aggregatedSignal struct {
	Name     string
	Type     string
	Count    int
	LatestAt string
}

// buildSignalStats 按概念组把推荐和风险合并为每个标的一行。
// 输入：rows 是目标天数内分析行，groups 是已持久化的概念组和别名。
// 输出：按总次数和最近出现时间倒序返回信号榜。
// 副作用：无。
func buildSignalStats(rows []analysisRow, groups []SignalGroup) []SignalStat {
	// 1. 建立别名索引，并沿用旧服务顺序分别聚合推荐、风险。
	groupIndex := buildSignalGroupIndex(groups)
	recommendations := aggregateSignals(rows, true)
	risks := aggregateSignals(rows, false)

	// 2. 推荐聚合项先进入概念榜，风险项补充计数或追加纯风险概念。
	grouped := make(map[string]*signalAccumulator)
	ordered := make([]*signalAccumulator, 0, len(recommendations)+len(risks))
	merge := func(signals []aggregatedSignal, recommendation bool) {
		for _, signal := range signals {
			groupKey := "pending"
			groupName, groupType := pendingSignalGroupName, pendingSignalGroupType
			if group, exists := groupIndex[normalizeSignalAlias(signal.Name)]; exists {
				groupKey = fmt.Sprintf("group:%d:%s", group.ID, normalizeSignalAlias(group.Name))
				groupName = group.Name
				if strings.TrimSpace(group.Type) != "" {
					groupType = group.Type
				}
			}
			item, exists := grouped[groupKey]
			if !exists {
				item = &signalAccumulator{
					SignalStat: SignalStat{
						Name:            groupName,
						Type:            groupType,
						Members:         []string{},
						MemberNetCounts: map[string]int{},
					},
					LatestAt: signal.LatestAt, memberKeys: make(map[string]struct{}),
				}
				grouped[groupKey] = item
				ordered = append(ordered, item)
			}
			memberKey := strings.TrimSpace(signal.Name)
			if _, exists := item.memberKeys[memberKey]; !exists {
				item.memberKeys[memberKey] = struct{}{}
				item.Members = append(item.Members, strings.TrimSpace(signal.Name))
			}
			if item.Type == "other" && signal.Type != "" && signal.Type != "other" {
				item.Type = signal.Type
			}
			if recommendation {
				item.RecommendationCount += signal.Count
				item.MemberNetCounts[memberKey] += signal.Count
			} else {
				item.RiskCount += signal.Count
				item.MemberNetCounts[memberKey] -= signal.Count
			}
			item.Count += signal.Count
			if signal.LatestAt > item.LatestAt {
				item.LatestAt = signal.LatestAt
			}
		}
	}
	merge(recommendations, true)
	merge(risks, false)

	// 3. 稳定排序保证完全相同的总数和日期保留聚合插入顺序。
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Name == pendingSignalGroupName || ordered[right].Name == pendingSignalGroupName {
			return ordered[right].Name == pendingSignalGroupName && ordered[left].Name != pendingSignalGroupName
		}
		if ordered[left].Count == ordered[right].Count {
			return ordered[left].LatestAt > ordered[right].LatestAt
		}
		return ordered[left].Count > ordered[right].Count
	})
	results := make([]SignalStat, 0, len(ordered))
	for _, item := range ordered {
		results = append(results, item.SignalStat)
	}
	return results
}

// buildSignalGroupIndex 把概念组别名转换为规范化名称索引。
// 输入：groups 是数据库读取的概念组和全部别名。
// 输出：返回原始名称到概念组的只读映射。
// 副作用：无。
func buildSignalGroupIndex(groups []SignalGroup) map[string]SignalGroup {
	// 1. 为每个非空别名登记所属概念组，重复别名保留首个有效配置。
	index := make(map[string]SignalGroup)
	for _, group := range groups {
		for _, alias := range group.Aliases {
			key := normalizeSignalAlias(alias)
			if key == "" {
				continue
			}
			if _, exists := index[key]; !exists {
				index[key] = group
			}
		}
	}
	return index
}

// normalizeSignalAlias 规范化别名查找键，不改变页面展示原文。
// 输入：name 是模型返回或数据库保存的原始标的名称。
// 输出：返回去除首尾空白并转为小写的稳定键。
// 副作用：无。
func normalizeSignalAlias(name string) string {
	// 1. 只处理无语义差异的大小写和首尾空白，近义词交给概念映射。
	return strings.ToLower(strings.TrimSpace(name))
}

// aggregateSignals 按名称和类型聚合推荐或风险信号。
// 输入：rows 是时间倒序分析行，recommendation 选择推荐或风险数组。
// 输出：按次数、最近日期倒序返回聚合项，完全同分时保留首次出现顺序。
// 副作用：无。
func aggregateSignals(rows []analysisRow, recommendation bool) []aggregatedSignal {
	// 1. 使用结构化键累计次数，并用切片保留仓储结果中的首次出现顺序。
	type signalKey struct {
		name string
		kind string
	}
	grouped := make(map[signalKey]*aggregatedSignal)
	ordered := make([]*aggregatedSignal, 0)
	for _, row := range rows {
		signals := row.Risks
		if recommendation {
			signals = row.Recommendations
		}
		for _, signal := range signals {
			name := strings.TrimSpace(signal.Name)
			if name == "" {
				continue
			}
			kind := strings.TrimSpace(signal.Type)
			if kind == "" {
				kind = "other"
			}
			key := signalKey{name: name, kind: kind}
			item, exists := grouped[key]
			if !exists {
				item = &aggregatedSignal{Name: name, Type: kind, LatestAt: row.OccurredAt}
				grouped[key] = item
				ordered = append(ordered, item)
			}
			item.Count++
			if row.OccurredAt > item.LatestAt {
				item.LatestAt = row.OccurredAt
			}
		}
	}

	// 2. 稳定排序复刻旧服务按次数和最近日期倒序的聚合列表。
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Count == ordered[right].Count {
			return ordered[left].LatestAt > ordered[right].LatestAt
		}
		return ordered[left].Count > ordered[right].Count
	})
	results := make([]aggregatedSignal, 0, len(ordered))
	for _, item := range ordered {
		results = append(results, *item)
	}
	return results
}

// buildDistribution 统计市场氛围或涨跌预测分布。
// 输入：rows 是市场天数内分析行，mood 控制统计字段。
// 输出：按次数倒序、名称正序返回分布。
// 副作用：无。
func buildDistribution(rows []analysisRow, mood bool) []DistributionItem {
	// 1. 规范化枚举并累计次数，同时记录仓储倒序结果中的首次出现顺序。
	counts := make(map[string]int)
	firstSeen := make(map[string]int)
	for _, row := range rows {
		key := normalizePrediction(row.Prediction)
		if mood {
			key = normalizeMood(row.MarketMood)
		}
		if _, exists := counts[key]; !exists {
			firstSeen[key] = len(firstSeen)
		}
		counts[key]++
	}
	results := make([]DistributionItem, 0, len(counts))
	for name, count := range counts {
		results = append(results, DistributionItem{Name: name, Count: count})
	}

	// 2. 保持高频项优先，同次数时沿用旧接口的首次出现顺序。
	sort.Slice(results, func(left, right int) bool {
		if results[left].Count == results[right].Count {
			return firstSeen[results[left].Name] < firstSeen[results[right].Name]
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
7. 剔除纯结果导向的信息：例如“科技大涨虹吸传统行业”“年初至今盈利30-40%%”“涨幅领先”“一枝独秀”等已经发生的涨跌、排名、收益结果。
8. 只有当文章给出面向未来的理由时才抽取标的，例如估值、盈利/业绩、政策、供需、周期、库存、订单、流动性、风险事件、配置价值、催化或基本面变化。
9. 如果一句话只是描述当前涨跌、过去收益、资金当下流向，而没有未来判断，请不要抽取为 recommendations 或 risks。

JSON 结构：
{
  "summary": "80字以内中文摘要",
  "recommendations": [
    {
      "name": "标的名称",
      "type": "stock|sector|index|commodity|crypto|concept|other",
      "reason": "80字以内综合原因，说明为什么最终偏推荐"
    }
  ],
  "risks": [
    {
      "name": "标的名称",
      "type": "stock|sector|index|commodity|crypto|concept|other",
      "reason": "80字以内综合原因，说明为什么最终偏风险"
    }
  ],
  "market": {
    "mood": "very_optimistic|optimistic|neutral|pessimistic|very_pessimistic|unknown",
    "mood_reason": "80字以内原因，说明文章为什么体现这种短期市场氛围",
    "prediction": "up|down|range|unknown",
    "prediction_reason": "80字以内原因，说明文章为什么体现这种短期涨跌预测"
  }
}

来源：%s
来源类型：%s
标题：%s
链接：%s
发布时间：%s

文章内容：
%s`, article.SourceName, article.SourceType, article.Title, article.Link, article.PublishedAt, content)
}
