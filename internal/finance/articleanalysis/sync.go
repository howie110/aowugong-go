package articleanalysis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
)

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
