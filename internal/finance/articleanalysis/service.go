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

type forceArticleFetchContextKey struct{}

const (
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
	Model                  string
	FeedURL                string
	Articles               ArticleGateway
	WeRead                 *WeReadSource
	Analyzer               AnalysisGateway
	AnalysisModels         []AnalysisModelConfig
	DefaultAnalysisModelID string
	Now                    func() time.Time
}

// SyncNow 立即抓取全部启用文章来源，供页面手动按钮使用。
// 输入：ctx 控制处理，fetchLimit 是每来源上限，analyze 控制是否分析，analysisLimit 是分析上限。
// 输出：返回来源、抓取、写入和分析统计；基础数据库失败时返回错误。
// 副作用：立即读取外部文章，按需调用当前分析模型，并写入 PostgreSQL。
func (s *Service) SyncNow(ctx context.Context, fetchLimit int, analyze bool, analysisLimit int) (SyncResult, error) {
	// 1. 通过上下文标记跳过公众号定时频率判断，正式定时任务仍走原有节流逻辑。
	return s.Sync(context.WithValue(ctx, forceArticleFetchContextKey{}, true), fetchLimit, analyze, analysisLimit)
}

// Sync 抓取全部启用来源，并按选项继续分析待处理文章。
// 输入：ctx 控制处理，fetchLimit 是每来源上限，analyze 控制是否分析，analysisLimit 是分析上限。
// 输出：返回来源、抓取、写入和分析统计；基础数据库失败时返回错误。
// 副作用：读取外部文章，按需调用当前分析模型，并写入 PostgreSQL。
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

	// 2. 逐来源读取外部数据并写入新文章，部分成功项也必须正常落库。
	for _, source := range sources {
		if s.options.Articles == nil {
			message := "文章客户端未配置"
			_ = s.repository.UpdateSourceStatus(ctx, source.ID, "error", message)
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": message})
			continue
		}
		feedURL := source.FeedURL
		if source.SourceType == "miniflux" && s.options.FeedURL != "" {
			feedURL = s.options.FeedURL
		}
		items, fetchErr := s.options.Articles.Fetch(ctx, source.ID, feedURL, fetchLimit)
		if fetchErr != nil {
			result.FailedSources = append(result.FailedSources, map[string]string{"source": source.SourceName, "error": fetchErr.Error()})
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
		status := "success"
		message := fmt.Sprintf("fetched=%d, inserted=%d, updated=%d, unchanged=%d", len(items), inserted, updated, unchanged)
		if fetchErr != nil {
			status, message = "error", fetchErr.Error()
		}
		if err := s.repository.UpdateSourceStatus(ctx, source.ID, status, message); err != nil {
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
		result.ClassifiedAliasCount = analysisResult.ClassifiedAliasCount
		result.SkippedCount = analysisResult.SkippedCount
		result.ErrorCount = analysisResult.ErrorCount
	}
	return result, nil
}

// ParsePending 解析已经抓取元数据但尚未成功获取正文的文章。
// 输入：ctx 控制请求，limit 限制本次解析数量。
// 输出：返回解析统计；失败文章保留待解析状态。
// 副作用：调用微信原文接口并写入 PostgreSQL。
func (s *Service) ParsePending(ctx context.Context, limit int) (ParseBatchResult, error) {
	// 1. 解析功能只对当前微信读书来源开放，避免误调用其他文章来源。
	if s.options.WeRead == nil {
		return ParseBatchResult{}, fmt.Errorf("微信读书解析器未配置")
	}
	items, err := s.repository.pendingParseArticles(ctx, limit)
	if err != nil {
		return ParseBatchResult{}, err
	}
	result := ParseBatchResult{Items: make([]map[string]any, 0, len(items))}
	credentials, credentialErr := s.options.WeRead.loadCredentials(ctx)
	if credentialErr != nil {
		return result, credentialErr
	}
	originalCredentials := credentials
	for _, item := range items {
		content, fetchErr := s.parseWeReadArticle(ctx, item, &credentials)
		if fetchErr != nil || strings.TrimSpace(content) == "" {
			result.ErrorCount++
			result.Items = append(result.Items, map[string]any{"id": item.ID, "title": item.Title, "status": "pending_parse", "error": errorText(fetchErr, "正文为空")})
			continue
		}
		if err := s.repository.UpdateArticleContent(ctx, item.ID, content, truncateRunes(content, 300), "parsed"); err != nil {
			return result, err
		}
		result.ParsedCount++
		result.Items = append(result.Items, map[string]any{"id": item.ID, "title": item.Title, "status": "parsed"})
	}
	if credentials != originalCredentials {
		if saveErr := s.options.WeRead.saveCredentials(ctx, credentials, false); saveErr != nil {
			return result, saveErr
		}
	}
	return result, nil
}

// parseWeReadArticle 优先读取微信读书详情正文，再访问微信公众号原文。
// 输入：ctx 控制请求，item 是待解析文章，credentials 是可自动刷新的凭据。
// 输出：返回正文或带上下文的失败原因。
// 副作用：调用微信读书和微信公众号接口，可能刷新凭据。
func (s *Service) parseWeReadArticle(ctx context.Context, item pendingParseArticle, credentials *client.WeReadArticleCredentials) (string, error) {
	// 1. 读取抓取时保存的 review_id，避免直接访问已经触发环境验证的微信原文。
	var raw struct {
		ReviewID string `json:"review_id"`
	}
	if strings.TrimSpace(item.RawEntryJSON) != "" {
		if err := json.Unmarshal([]byte(item.RawEntryJSON), &raw); err != nil {
			return "", fmt.Errorf("解析文章原始标识: %w", err)
		}
	}
	if raw.ReviewID != "" {
		detail, err := s.options.WeRead.client.FetchArticleDetail(ctx, credentials, raw.ReviewID)
		if err == nil && strings.TrimSpace(detail.Content) != "" {
			return detail.Content, nil
		}
	}

	// 2. 详情接口没有正文时，再用 Readability 解析微信公众号原文。
	content, _, err := s.options.WeRead.client.FetchArticleContent(ctx, item.Link)
	if err != nil {
		return "", err
	}
	return content, nil
}

func errorText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// SyncScheduled 执行生产任务使用的完整抓取和分批分析流程。
// 输入：ctx 控制处理，classifySignals 控制是否补齐六十天信号概念映射。
// 输出：返回累计同步统计；来源失败、模型缺失或仍有待分析文章时返回错误。
// 副作用：调用微信读书、微信公众号原文、当前分析模型，并写入 PostgreSQL。
func (s *Service) SyncScheduled(ctx context.Context, classifySignals bool) (SyncResult, error) {
	// 1. 抓取全部来源的当前文章，来源失败时保留明细并立即升级为任务错误。
	result, err := s.Sync(ctx, scheduledFetchLimit, false, 0)
	if err != nil {
		return result, fmt.Errorf("抓取投资文章: %w", err)
	}
	fetchFailure := ""
	if len(result.FailedSources) > 0 {
		fetchFailure = formatFailedSources(result.FailedSources)
	}

	// 2. 读取抓取后 pending；模型未配置时保留数据并明确失败告警。
	if err := s.validateScheduledSourceFreshness(result); err != nil {
		return result, err
	}
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return result, fmt.Errorf("读取文章分析进度: %w", err)
	}
	result.PendingCount = counts.PendingCount
	if result.PendingCount > 0 {
		model, modelErr := s.selectedAnalysisModel(ctx)
		if modelErr != nil {
			return result, modelErr
		}
		if model.Analyzer == nil || !model.Analyzer.Configured() {
			return result, fmt.Errorf("未配置可用的文章分析模型，仍有 %d 篇投资文章等待分析", result.PendingCount)
		}
	}

	// 3. 最多执行十个五十篇批次，持续累计并在没有成功进展时停止重试。
	for batch := 0; batch < scheduledAnalysisMaxBatches && result.PendingCount > 0; batch++ {
		analysisResult, analyzeErr := s.AnalyzePending(ctx, scheduledAnalysisBatchLimit)
		if analyzeErr != nil {
			return result, fmt.Errorf("分析第 %d 批投资文章: %w", batch+1, analyzeErr)
		}
		result.AnalyzedCount += analysisResult.AnalyzedCount
		result.ClassifiedAliasCount += analysisResult.ClassifiedAliasCount
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
		if fetchFailure != "" {
			return result, fmt.Errorf("投资文章抓取存在失败来源: %s", fetchFailure)
		}
		return result, nil
	}
	classifiedCount, err := s.classifySignalAliases(ctx, DefaultTargetDays)
	if err != nil {
		return result, fmt.Errorf("归类投资信号: %w", err)
	}
	result.ClassifiedAliasCount += classifiedCount
	if fetchFailure != "" {
		return result, fmt.Errorf("投资文章抓取存在失败来源: %s", fetchFailure)
	}
	return result, nil
}

// validateScheduledSourceFreshness 检查本次新发现的微信读书文章是否已经长期过旧。
// 输入：result 是本次增量读取统计，包含新发现文章的最新发布时间。
// 输出：新发现文章超过固定阈值时返回错误；没有新文章时返回 nil。
// 副作用：无，不访问数据库、不发送通知。
func (s *Service) validateScheduledSourceFreshness(result SyncResult) error {
	// 1. 微信读书来源只返回数据库未知文章，本次为零表示没有增量而不是上游故障。
	if strings.TrimSpace(result.LatestFetchedAt) == "" {
		return nil
	}
	latest, err := time.Parse("2006-01-02 15:04:05", result.LatestFetchedAt)
	if err != nil {
		return nil
	}

	// 2. 有发布时间时使用可注入时钟计算滞后，超过三天交给任务包装器失败通知。
	now := time.Now
	if s.options.Now != nil {
		now = s.options.Now
	}
	lag := now().UTC().Sub(latest.UTC())
	if lag > scheduledSourceStaleAfter {
		return fmt.Errorf("微信读书上游最新文章过旧: 最新=%s, 已滞后=%s, 请检查公众号更新状态", result.LatestFetchedAt, formatDurationHours(lag))
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
// 副作用：调用当前分析模型并写入 PostgreSQL 分析表。
func (s *Service) AnalyzePending(ctx context.Context, limit int) (AnalysisBatchResult, error) {
	// 1. 读取待分析文章并准备非 nil 结果数组。
	articles, err := s.repository.pendingArticles(ctx, limit)
	if err != nil {
		return AnalysisBatchResult{}, fmt.Errorf("读取待分析文章: %w", err)
	}
	result := AnalysisBatchResult{Items: []map[string]any{}}
	if len(articles) == 0 {
		return result, nil
	}
	model, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return AnalysisBatchResult{}, err
	}
	groups, err := s.repository.SignalGroups(ctx)
	if err != nil {
		return AnalysisBatchResult{}, err
	}

	// 2. 每篇使用当时最新概念组分析和落库，模型错误不阻断下一篇。
	for _, article := range articles {
		item, status, classifiedCount, err := s.analyzeOne(ctx, article, model, groups)
		if err != nil {
			return AnalysisBatchResult{}, err
		}
		result.Items = append(result.Items, item)
		result.ClassifiedAliasCount += classifiedCount
		if classifiedCount > 0 {
			groups, err = s.repository.SignalGroups(ctx)
			if err != nil {
				return AnalysisBatchResult{}, err
			}
		}
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

// AnalyzeAllPending 连续分析待处理文章，供页面手动补齐历史文章使用。
// 输入：ctx 控制处理；每批固定五十篇，最多处理十批。
// 输出：返回所有已执行批次的累计结果；数据库失败时返回错误。
// 副作用：调用当前分析模型并写入 PostgreSQL 分析表。
func (s *Service) AnalyzeAllPending(ctx context.Context) (AnalysisBatchResult, error) {
	// 1. 循环复用单批入口，遇到没有进展时停止，避免错误状态反复占用模型额度。
	result := AnalysisBatchResult{Items: []map[string]any{}}
	for batch := 0; batch < scheduledAnalysisMaxBatches; batch++ {
		current, err := s.AnalyzePending(ctx, scheduledAnalysisBatchLimit)
		if err != nil {
			return AnalysisBatchResult{}, err
		}
		result.AnalyzedCount += current.AnalyzedCount
		result.ClassifiedAliasCount += current.ClassifiedAliasCount
		result.SkippedCount += current.SkippedCount
		result.ErrorCount += current.ErrorCount
		result.Items = append(result.Items, current.Items...)
		if current.AnalyzedCount == 0 || len(current.Items) < scheduledAnalysisBatchLimit {
			break
		}
	}
	return result, nil
}

// AnalyzeAndClassifyPending 分析待处理文章并补齐六十天信号概念映射。
// 输入：ctx 控制处理，limit 是单批上限，all 控制是否连续处理全部待分析文章。
// 输出：返回文章分析和新增信号别名数量；分析或归类失败时返回错误。
// 副作用：调用当前分析模型并写入文章分析、信号概念组和别名表。
func (s *Service) AnalyzeAndClassifyPending(ctx context.Context, limit int, all bool) (AnalysisBatchResult, error) {
	// 1. 页面选择全部时复用多批入口，否则只处理指定上限。
	var result AnalysisBatchResult
	var err error
	if all {
		result, err = s.AnalyzeAllPending(ctx)
	} else {
		result, err = s.AnalyzePending(ctx, limit)
	}
	if err != nil {
		return result, err
	}

	// 2. 即使没有待分析文章，也补扫统计窗口内尚未映射的信号名称。
	classifiedCount, err := s.classifySignalAliases(ctx, DefaultTargetDays)
	if err != nil {
		return result, fmt.Errorf("归类投资信号: %w", err)
	}
	result.ClassifiedAliasCount += classifiedCount
	return result, nil
}

// analyzeOne 分析单篇文章并持久化最终状态。
// 输入：ctx 控制模型请求，article 是待分析文章，groups 是当前概念词典。
// 输出：返回页面结果项、业务状态、新增别名数和仅数据库失败时使用的错误。
// 副作用：调用当前分析模型并写入 PostgreSQL 分析及概念映射表。
func (s *Service) analyzeOne(ctx context.Context, article pendingArticle, model analysisModelRuntime, groups []SignalGroup) (map[string]any, string, int, error) {
	// 1. 未配置模型时保留 pending，便于配置后重试。
	if model.Analyzer == nil || !model.Analyzer.Configured() {
		message := "未配置可用的文章分析模型"
		if err := s.repository.SaveAnalysis(ctx, article.ID, "pending", AnalysisResult{}, message, model.Model, PromptVersion); err != nil {
			return nil, "", 0, err
		}
		return map[string]any{"article_id": article.ID, "status": "skipped", "message": message}, "skipped", 0, nil
	}

	// 2. 同一次模型调用完成文章分析和每个标的的概念组决策。
	content, err := model.Analyzer.SimpleChat(ctx, buildAnalysisPrompt(article, groups), 2400)
	if err != nil {
		message := err.Error()
		if saveErr := s.repository.SaveAnalysis(ctx, article.ID, "error", AnalysisResult{}, message, model.Model, PromptVersion); saveErr != nil {
			return nil, "", 0, saveErr
		}
		return map[string]any{"article_id": article.ID, "status": "error", "message": message}, "error", 0, nil
	}
	parsed, err := parseAnalysisJSON(content)
	if err != nil {
		message := "模型 JSON 解析失败：" + err.Error()
		if saveErr := s.repository.SaveAnalysis(ctx, article.ID, "error", AnalysisResult{}, message, model.Model, PromptVersion); saveErr != nil {
			return nil, "", 0, saveErr
		}
		return map[string]any{"article_id": article.ID, "status": "error", "message": message}, "error", 0, nil
	}
	normalized := NormalizeAnalysis(parsed)
	candidates := analysisSignalCandidates(normalized)
	classification, err := validateSignalClassificationPayload(
		signalClassificationPayload{Decisions: filterSignalClassificationDecisions(parsed.SignalClassifications, parsed, candidates)},
		candidates, groups,
	)
	if err != nil {
		message := "模型信号归类失败：" + err.Error()
		if saveErr := s.repository.SaveAnalysis(ctx, article.ID, "error", normalized, message, model.Model, PromptVersion); saveErr != nil {
			return nil, "", 0, saveErr
		}
		return map[string]any{"article_id": article.ID, "status": "error", "message": message}, "error", 0, nil
	}
	classifiedCount := 0
	groupsToSave := classification.Groups
	if len(groupsToSave) > 0 {
		classifiedCount, err = s.repository.SaveSignalGroups(ctx, groupsToSave, model.Model)
		if err != nil {
			return nil, "", 0, err
		}
	}
	if err := s.repository.SaveAnalysis(ctx, article.ID, "success", normalized, "", model.Model, PromptVersion); err != nil {
		return nil, "", 0, err
	}
	return map[string]any{"article_id": article.ID, "status": "success", "classified_alias_count": classifiedCount}, "success", classifiedCount, nil
}

// analysisSignalCandidates 返回最终文章信号中的唯一分类候选。
func analysisSignalCandidates(result AnalysisResult) []signalCandidate {
	seen := make(map[string]struct{})
	candidates := make([]signalCandidate, 0, len(result.Recommendations)+len(result.Risks))
	appendSignals := func(signals []Signal) {
		for _, signal := range signals {
			key := normalizeSignalAlias(signal.Name)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, signalCandidate{Name: signal.Name, Type: signal.Type})
		}
	}
	appendSignals(result.Recommendations)
	appendSignals(result.Risks)
	return candidates
}

// filterSignalClassificationDecisions 丢弃规范化时已移除信号的对应决策，并保留真正的越权名称供校验拒绝。
func filterSignalClassificationDecisions(decisions []signalClassificationDecision, raw AnalysisResult, candidates []signalCandidate) []signalClassificationDecision {
	allowed := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		allowed[normalizeSignalAlias(candidate.Name)] = candidate.Name
	}
	rawNames := make(map[string]struct{}, len(raw.Recommendations)+len(raw.Risks))
	for _, signal := range append(append([]Signal{}, raw.Recommendations...), raw.Risks...) {
		rawNames[normalizeSignalAlias(compactSignalName(signal.Name))] = struct{}{}
	}
	result := make([]signalClassificationDecision, 0, len(decisions))
	for _, decision := range decisions {
		key := normalizeSignalAlias(compactSignalName(decision.Name))
		if normalizedName, exists := allowed[key]; exists {
			decision.Name = normalizedName
			result = append(result, decision)
			continue
		}
		if _, removedByNormalization := rawNames[key]; !removedByNormalization {
			result = append(result, decision)
		}
	}
	return result
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

// feedEntryFromClient 把通用外部文章模型转换为文章仓储模型。
// 输入：item 是客户端规范化文章。
// 输出：返回 repository 使用的 FeedEntry。
// 副作用：无。
func feedEntryFromClient(item client.ArticleItem) FeedEntry {
	// 1. 一一映射字段，业务包继续拥有存储模型。
	return FeedEntry{
		ArticleKey: item.ArticleKey, ExternalID: item.ExternalID, Title: item.Title,
		Link: item.Link, Author: item.Author, PublishedAt: item.PublishedAt,
		Summary: item.Summary, Content: item.Content, FetchStatus: item.FetchStatus, RawEntry: item.RawEntry,
	}
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
	// 1. 兼容只传旧 Analyzer 的测试和调用方，并构造稳定模型目录。
	if options.Model == "" {
		options.Model = "deepseek-v4-pro"
	}
	options.FeedURL = strings.TrimSpace(options.FeedURL)
	models := append([]AnalysisModelConfig(nil), options.AnalysisModels...)
	if len(models) == 0 && options.Analyzer != nil {
		models = append(models, AnalysisModelConfig{
			ID: "legacy:" + options.Model, Provider: "deepseek", Model: options.Model,
			Label: options.Model, Analyzer: options.Analyzer,
		})
	}
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

// AnalysisModelSettings 返回当前模型选择和完整模型目录。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回当前有效模型及各模型配置状态。
// 副作用：只读 PostgreSQL。
func (s *Service) AnalysisModelSettings(ctx context.Context) (AnalysisModelSettings, error) {
	// 1. 解析持久化选择；无设置或旧设置无效时使用当前默认模型。
	selected, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return AnalysisModelSettings{}, err
	}

	// 2. 按配置顺序输出目录，未配置 Key 的保留显示但不可选择。
	choices := make([]AnalysisModelChoice, 0, len(s.analysisModelOrder))
	for _, id := range s.analysisModelOrder {
		model := s.analysisModels[id]
		choices = append(choices, AnalysisModelChoice{
			ID: id, Provider: model.Provider, Model: model.Model, Label: model.Label,
			Configured: model.Analyzer.Configured(),
		})
	}
	return AnalysisModelSettings{
		SelectedModelID: selected.ID, SelectedModel: selected.Model, Models: choices,
	}, nil
}

// SetAnalysisModel 保存后续文章分析和信号归类使用的模型。
// 输入：ctx 控制写入，modelID 必须来自当前模型目录且已配置。
// 输出：返回更新后的模型设置；模型不可用时返回错误。
// 副作用：写入 PostgreSQL 模型设置。
func (s *Service) SetAnalysisModel(ctx context.Context, modelID string) (AnalysisModelSettings, error) {
	// 1. 拒绝目录外或缺少凭据的模型，避免页面保存一个必然失败的选项。
	model, exists := s.analysisModels[strings.TrimSpace(modelID)]
	if !exists {
		return AnalysisModelSettings{}, fmt.Errorf("文章分析模型不存在")
	}
	if !model.Analyzer.Configured() {
		return AnalysisModelSettings{}, fmt.Errorf("文章分析模型 %s 尚未配置凭据", model.Label)
	}
	if err := s.repository.SaveAnalysisModelID(ctx, model.ID); err != nil {
		return AnalysisModelSettings{}, err
	}
	return s.AnalysisModelSettings(ctx)
}

// selectedAnalysisModel 解析当前持久化选择并回退到第一个已配置模型。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回本次调用固定使用的模型运行时；没有模型目录时返回空运行时。
// 副作用：只读 PostgreSQL。
func (s *Service) selectedAnalysisModel(ctx context.Context) (analysisModelRuntime, error) {
	// 1. 单模型兼容模式无需额外查询，避免改变旧测试和简单调用行为。
	if len(s.analysisModelOrder) == 1 {
		return s.analysisModels[s.analysisModelOrder[0]], nil
	}
	selectedID := ""
	if len(s.analysisModelOrder) > 1 {
		var err error
		selectedID, err = s.repository.AnalysisModelID(ctx)
		if err != nil {
			return analysisModelRuntime{}, err
		}
	}
	if selected, exists := s.analysisModels[selectedID]; exists && selected.Analyzer.Configured() {
		return selected, nil
	}
	if fallback, exists := s.analysisModels[s.defaultAnalysisModelID]; exists && fallback.Analyzer.Configured() {
		return fallback, nil
	}
	for _, id := range s.analysisModelOrder {
		if candidate := s.analysisModels[id]; candidate.Analyzer.Configured() {
			return candidate, nil
		}
	}
	if fallback, exists := s.analysisModels[s.defaultAnalysisModelID]; exists {
		return fallback, nil
	}
	if len(s.analysisModelOrder) > 0 {
		return s.analysisModels[s.analysisModelOrder[0]], nil
	}
	return analysisModelRuntime{}, nil
}

// AnalysisSummary 构建投资文章分析页面摘要。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回文章和已分析计数；失败时返回错误。
// 副作用：只读 PostgreSQL。
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
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "结构化模型结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// FetchSummary 构建投资文章抓取页面摘要。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回来源、文章、待分析和已分析计数；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) FetchSummary(ctx context.Context) (PageSummary, error) {
	// 1. 读取统一计数口径。
	counts, err := s.repository.counts(ctx)
	if err != nil {
		return PageSummary{}, fmt.Errorf("读取文章抓取摘要: %w", err)
	}

	// 2. 组装抓取页状态卡片。
	return PageSummary{
		Title:       "投资文章抓取",
		Description: "管理微信读书公众号，读取新文章并触发结构化模型分析。",
		Metrics: []PageMetric{
			{Label: "来源", Value: strconv.Itoa(counts.SourceCount), Detail: "启用的信息源", Status: "normal"},
			{Label: "文章", Value: strconv.Itoa(counts.ArticleCount), Detail: "已入库文章", Status: "normal"},
			{Label: "待分析", Value: strconv.Itoa(counts.PendingCount), Detail: "抓取后等待模型处理", Status: "normal"},
			{Label: "已分析", Value: strconv.Itoa(counts.AnalyzedCount), Detail: "结构化模型结果", Status: "normal"},
		},
		LatestArticleAt: counts.LatestAt,
	}, nil
}

// Sources 返回页面展示的信息源列表。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回全部来源；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Sources(ctx context.Context) ([]Source, error) {
	// 1. 页面需要同时看到未配置来源状态。
	return s.repository.Sources(ctx, false)
}

// Articles 返回指定天数内已分析文章。
// 输入：ctx 控制查询，days 和 limit 限制范围。
// 输出：返回文章列表；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Articles(ctx context.Context, days, limit int) ([]ArticleItem, error) {
	// 1. 复用仓储层受限查询。
	return s.repository.Articles(ctx, days, limit)
}

// Detail 返回单篇文章详情。
// 输入：ctx 控制查询，articleID 是文章主键。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：只读 PostgreSQL。
func (s *Service) Detail(ctx context.Context, articleID int64) (*ArticleDetail, error) {
	// 1. 复用仓储层详情映射。
	return s.repository.Detail(ctx, articleID)
}

// UpdatePromptFeedback 保存管理员修正意见并返回最新详情。
// 输入：ctx 控制写入，articleID 是文章主键，feedback 是修正意见。
// 输出：返回详情或 nil；失败时返回错误。
// 副作用：写入 PostgreSQL。
func (s *Service) UpdatePromptFeedback(ctx context.Context, articleID int64, feedback string) (*ArticleDetail, error) {
	// 1. 由仓储层统一截断并更新反馈。
	return s.repository.UpdatePromptFeedback(ctx, articleID, feedback)
}

// Report 构建信号榜和短期市场分布。
// 输入：ctx 控制查询，targetDays 默认 60，marketDays 默认 3。
// 输出：返回完整分析报告；失败时返回错误。
// 副作用：只读 PostgreSQL。
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

	// 2. 聚合推荐、风险及市场枚举并附带当前模型说明。
	model, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return Report{}, err
	}
	return Report{
		AnalysisModel:          model.Model,
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
				// 已持久化的待归类别名与尚未映射信号必须进入同一特殊分组。
				if group.Name != pendingSignalGroupName && group.Type != pendingSignalGroupType {
					groupKey = fmt.Sprintf("group:%d:%s", group.ID, normalizeSignalAlias(group.Name))
					groupName = group.Name
					if strings.TrimSpace(group.Type) != "" {
						groupType = group.Type
					}
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
	}, []SignalGroup{{ID: 123, Name: "{现有概念组名称}", Type: "sector"}})
}

// buildAnalysisPrompt 根据文章内容生成结构化分析提示词。
// 输入：article 是待分析文章，groups 是当前允许复用的概念组。
// 输出：返回严格 JSON 输出约束提示词。
// 副作用：无。
func buildAnalysisPrompt(article pendingArticle, groups []SignalGroup) string {
	// 1. 正文为空时回退摘要并限制最大字符数。
	content := article.Content
	if content == "" {
		content = article.Summary
	}
	content = truncateRunes(content, 12000)
	groupOptions := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		if group.Name == pendingSignalGroupName || group.Type == pendingSignalGroupType {
			continue
		}
		groupOptions = append(groupOptions, map[string]any{"id": group.ID, "name": group.Name, "type": group.Type})
	}
	groupsJSON, _ := json.Marshal(groupOptions)

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

概念组归类规则：
1. 在同一次返回中，为 recommendations 和 risks 最终保留的每个标的各返回一条 signal_classifications 决策；没有标的时返回空数组。
2. 每条决策必须选择 reuse、create 两种 action 之一，系统不允许 pending 或待归类；name 必须与信号列表中的 name 完全一致。
3. 优先 reuse，并通过 existing_group_id 引用下方已有概念组；只有确实没有合适概念组时才 create。
4. 具体公司应上卷到最直接的行业或主题，例如证券公司归入“证券行业”。相关但统计含义不同的主题不要过度合并。
5. create 的 canonical_name 必须是简洁、通行、单一的行业、主题、资产或市场名称，禁止用斜杠拼接；type 只能是 sector、concept、company、commodity、index、market、crypto、other。
6. 信息不足时仍必须明确归类：通用策略归入“投资策略”，无法识别的公司归入“未具名公司”，其他无法辨认的名称归入“信息不明确”；必要时 create。confidence 范围是 0 到 1，只记录可信度。
7. 每个最终标的必须且只能有一条决策，不得返回信号列表之外的名称。

已有概念组（待归类组已排除，只能使用这里列出的 id）：
%s

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
  "signal_classifications": [
    {
      "name": "与 recommendations 或 risks 完全一致的标的名称",
      "action": "reuse|create",
      "existing_group_id": 123,
      "canonical_name": "仅 create 时填写的新概念组名称",
      "type": "仅 create 时填写的概念组类型",
      "confidence": 0.96
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
%s`, string(groupsJSON), article.SourceName, article.SourceType, article.Title, article.Link, article.PublishedAt, content)
}
