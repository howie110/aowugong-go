package articleanalysis

import (
	"context"
	"fmt"
)

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

	// 2. 正常只调用一次；模型返回损坏 JSON 时在同一篇文章内有限重试。
	parsed, err := s.requestArticleAnalysis(ctx, article, groups, model)
	if err != nil {
		message := err.Error()
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
		// 3. 文章 JSON 已可用但分类决策漏项时，仅重做分类，避免整篇文章报废。
		if len(candidates) > 0 {
			if repaired, repairErr := s.classifySignalBatch(ctx, groups, candidates, model); repairErr == nil {
				classification, err = repaired, nil
			} else {
				err = fmt.Errorf("%w；补分类仍失败: %v", err, repairErr)
			}
		}
	}
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

// requestArticleAnalysis 调用文章分析模型并解析结构化结果。
// 输入：ctx 控制请求，article/groups 提供提示词数据，model 是当前 DeepSeek 模型。
// 输出：返回已解析结果；上游错误或连续无效 JSON 时返回可定位错误。
// 副作用：正常调用一次，异常响应最多再调用一次当前模型。
func (s *Service) requestArticleAnalysis(ctx context.Context, article pendingArticle, groups []SignalGroup, model analysisModelRuntime) (AnalysisResult, error) {
	basePrompt := buildAnalysisPrompt(article, groups)
	prompt := basePrompt
	var lastErr error
	for attempt := 1; attempt <= articleAnalysisMaxAttempts; attempt++ {
		content, err := model.Analyzer.SimpleChat(ctx, prompt, articleAnalysisMaxTokens)
		if err == nil {
			parsed, parseErr := parseAnalysisJSON(content)
			if parseErr == nil {
				return parsed, nil
			}
			lastErr = fmt.Errorf("模型 JSON 解析失败: %w", parseErr)
		} else {
			lastErr = fmt.Errorf("请求模型失败: %w", err)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AnalysisResult{}, fmt.Errorf("文章分析取消: %w", ctxErr)
		}
		prompt = basePrompt + `

这是同一篇文章的重试请求。上一次响应未通过校验，请重新完整输出一个可直接 JSON.parse 的 JSON 对象：
- 不要 Markdown、注释或任何 JSON 之外的文字；
- 最终 recommendations 和 risks 中每个标的都必须在 signal_classifications 中出现且只出现一次；
- 即使无法判断，也必须使用信息不明确等明确概念组完成分类；
- 输出最后一个字符必须是 }，不要尾逗号，也不要多余闭合符号。`
	}
	return AnalysisResult{}, fmt.Errorf("模型 %s 连续 %d 次返回无效文章分析: %w", model.Model, articleAnalysisMaxAttempts, lastErr)
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
