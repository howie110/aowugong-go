package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const signalClassificationConfidenceThreshold = 0.80

const signalClassificationBatchSize = 20

const signalClassificationMaxAttempts = 2

type signalCandidate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type signalAliasProposal struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

type signalGroupProposal struct {
	CanonicalName string                `json:"canonical_name"`
	Type          string                `json:"type"`
	Aliases       []signalAliasProposal `json:"aliases"`
}

type signalClassificationPayload struct {
	Groups []signalGroupProposal `json:"groups"`
}

// classifySignalAliases 使用 DeepSeek 批量归类指定范围内的未知信号名称。
// 输入：ctx 控制数据库和模型调用，days 是至少一天的文章范围。
// 输出：返回本次新增别名数量；查询、分类或写入失败时返回错误。
// 副作用：调用 DeepSeek，并写入 MySQL 概念组和别名表。
func (s *Service) classifySignalAliases(ctx context.Context, days int) (int, error) {
	// 1. 读取统计范围、已有概念词典并提取未知名称。
	if days < 1 {
		days = DefaultTargetDays
	}
	rows, err := s.repository.analysisRows(ctx, days)
	if err != nil {
		return 0, fmt.Errorf("读取待分类投资信号: %w", err)
	}
	groups, err := s.repository.SignalGroups(ctx)
	if err != nil {
		return 0, err
	}
	candidates := collectUnknownSignalCandidates(rows, groups)
	if len(candidates) == 0 {
		return 0, nil
	}
	if s.options.Analyzer == nil || !s.options.Analyzer.Configured() {
		return 0, fmt.Errorf("未配置 DEEPSEEK_API_KEY，仍有 %d 个投资信号等待分类", len(candidates))
	}

	// 2. 分批分类，每批写入后刷新词典供下一批优先复用。
	inserted := 0
	for start := 0; start < len(candidates); start += signalClassificationBatchSize {
		end := start + signalClassificationBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		proposals, err := s.classifySignalBatch(ctx, groups, batch)
		if err != nil {
			return inserted, fmt.Errorf("处理第 %d 批投资信号分类: %w", start/signalClassificationBatchSize+1, err)
		}
		count, err := s.repository.SaveSignalGroups(ctx, proposals, s.options.Model)
		if err != nil {
			return inserted, err
		}
		inserted += count
		if end < len(candidates) {
			groups, err = s.repository.SignalGroups(ctx)
			if err != nil {
				return inserted, err
			}
		}
	}
	return inserted, nil
}

// classifySignalBatch 调用 DeepSeek 分类单批未知名称，并重试畸形响应。
// 输入：ctx 控制模型调用，groups 是现有词典，batch 是本批未知名称。
// 输出：返回只含高置信度别名的分类；连续失败时返回最后一次错误。
// 副作用：最多调用两次 DeepSeek 外部接口。
func (s *Service) classifySignalBatch(ctx context.Context, groups []SignalGroup, batch []signalCandidate) ([]signalGroupProposal, error) {
	// 1. 固定本批提示词，避免重试时改变统计语义。
	prompt := buildSignalClassificationPrompt(groups, batch)
	var lastErr error

	// 2. 模型调用或 JSON 校验失败时重试一次，成功后立即返回稳定结果。
	for attempt := 1; attempt <= signalClassificationMaxAttempts; attempt++ {
		content, err := s.options.Analyzer.SimpleChat(ctx, prompt, 4000)
		if err == nil {
			proposals, parseErr := parseSignalClassificationJSON(content, batch)
			if parseErr == nil {
				return stabilizeSignalClassifications(proposals), nil
			}
			err = parseErr
		}
		lastErr = err
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("投资信号分类取消: %w", ctxErr)
		}
	}

	// 3. 保留最后一次业务上下文，便于任务失败通知定位具体原因。
	return nil, fmt.Errorf("DeepSeek 连续 %d 次返回无效分类: %w", signalClassificationMaxAttempts, lastErr)
}

// buildSignalClassificationPrompt 构造未知信号名称的严格分组提示词。
// 输入：groups 是已有概念词典，candidates 是本轮未知名称。
// 输出：返回要求完整 JSON 分类结果的中文提示词。
// 副作用：无。
func buildSignalClassificationPrompt(groups []SignalGroup, candidates []signalCandidate) string {
	// 1. 使用 JSON 向模型传递完整现有词典和当前候选，避免文本拼接歧义。
	groupsJSON, _ := json.Marshal(groups)
	candidatesJSON, _ := json.Marshal(candidates)

	// 2. 固定命名、上卷、复用和完整覆盖规则，确保报表口径稳定。
	return fmt.Sprintf(`你负责把投资文章里的原始标的名称归入稳定的统计概念组。

已有概念组：%s
本轮待分类名称：%s

规则：
1. 优先复用已有概念组；只有确实没有对应组时才创建新组。
2. 具体公司上卷到最直接的行业或主题，例如证券公司归入“证券行业”。
3. 规范概念名使用简洁、通行的行业、主题、资产或市场名称，只能有一个名称，禁止使用斜杠拼接多个名称。
4. 相关但统计含义不同的主题不要过度合并，选择与原始名称最直接的上级概念。
5. 每个待分类名称必须且只能出现一次，aliases.name 必须原样返回。
6. confidence 是 0 到 1 的归类置信度；不确定时如实给低分。

只返回以下结构的 JSON，不要解释，不要 Markdown：
{"groups":[{"canonical_name":"证券行业","type":"sector","aliases":[{"name":"中信证券","confidence":0.96}]}]}`,
		string(groupsJSON), string(candidatesJSON))
}

// collectUnknownSignalCandidates 收集尚未映射的去重原始信号名称。
// 输入：rows 是文章分析行，groups 是已有概念组。
// 输出：按首次出现顺序返回待分类名称和原始类型。
// 副作用：无。
func collectUnknownSignalCandidates(rows []analysisRow, groups []SignalGroup) []signalCandidate {
	// 1. 建立已有别名和本轮候选索引。
	mapped := buildSignalGroupIndex(groups)
	positions := make(map[string]int)
	result := make([]signalCandidate, 0)

	// 2. 按推荐、风险原始顺序收集未知名称，重复名称只保留一次。
	appendSignals := func(signals []Signal) {
		for _, signal := range signals {
			name := strings.TrimSpace(signal.Name)
			key := normalizeSignalAlias(name)
			if key == "" {
				continue
			}
			if _, exists := mapped[key]; exists {
				continue
			}
			kind := strings.TrimSpace(signal.Type)
			if kind == "" {
				kind = "other"
			}
			if position, exists := positions[key]; exists {
				if result[position].Type == "other" && kind != "other" {
					result[position].Type = kind
				}
				continue
			}
			positions[key] = len(result)
			result = append(result, signalCandidate{Name: name, Type: kind})
		}
	}
	for _, row := range rows {
		appendSignals(row.Recommendations)
		appendSignals(row.Risks)
	}
	return result
}

// parseSignalClassificationJSON 解码并校验 DeepSeek 的概念分组结果。
// 输入：content 是模型文本，candidates 是本轮必须完整覆盖的名称。
// 输出：返回结构化概念组；遗漏、重复或越权名称会返回错误。
// 副作用：无。
func parseSignalClassificationJSON(content string, candidates []signalCandidate) ([]signalGroupProposal, error) {
	// 1. 兼容纯 JSON 和 Markdown fenced JSON。
	content = strings.TrimSpace(content)
	if matches := fencedJSONPattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}
	var payload signalClassificationPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, fmt.Errorf("解析信号分类 JSON: %w", err)
	}

	// 2. 只接受本轮候选，并把模型名称还原为数据库中的原始写法。
	candidateIndex := make(map[string]signalCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateIndex[normalizeSignalAlias(candidate.Name)] = candidate
	}
	seen := make(map[string]struct{}, len(candidates))
	result := make([]signalGroupProposal, 0, len(payload.Groups))
	for _, group := range payload.Groups {
		group.CanonicalName = strings.TrimSpace(group.CanonicalName)
		group.Type = strings.TrimSpace(group.Type)
		if err := validateSignalGroupName(group.CanonicalName); err != nil {
			return nil, err
		}
		if group.Type == "" {
			group.Type = "other"
		}
		aliases := make([]signalAliasProposal, 0, len(group.Aliases))
		for _, alias := range group.Aliases {
			key := normalizeSignalAlias(alias.Name)
			candidate, exists := candidateIndex[key]
			if !exists {
				return nil, fmt.Errorf("信号分类返回未请求名称 %q", alias.Name)
			}
			if _, exists := seen[key]; exists {
				return nil, fmt.Errorf("信号分类重复返回名称 %q", candidate.Name)
			}
			if alias.Confidence < 0 || alias.Confidence > 1 {
				return nil, fmt.Errorf("信号分类 %q 置信度超出 0 到 1", candidate.Name)
			}
			seen[key] = struct{}{}
			aliases = append(aliases, signalAliasProposal{Name: candidate.Name, Confidence: alias.Confidence})
		}
		if len(aliases) > 0 {
			group.Aliases = aliases
			result = append(result, group)
		}
	}

	// 3. 缺少任一候选都拒绝整批结果，避免写入半套分类。
	missing := make([]string, 0)
	for _, candidate := range candidates {
		if _, exists := seen[normalizeSignalAlias(candidate.Name)]; !exists {
			missing = append(missing, candidate.Name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("信号分类缺少名称: %s", strings.Join(missing, "、"))
	}
	return result, nil
}

// validateSignalGroupName 校验模型生成的单一规范概念名称。
// 输入：name 是已经去除首尾空白的候选名称。
// 输出：名称简洁且不含列表拼接符时返回 nil，否则返回错误。
// 副作用：无。
func validateSignalGroupName(name string) error {
	// 1. 拒绝空值、超长名称和用于拼接多个名称的分隔符。
	if name == "" {
		return fmt.Errorf("信号分类包含空概念组名称")
	}
	if utf8.RuneCountInString(name) > 20 {
		return fmt.Errorf("信号分类概念组名称过长: %q", name)
	}
	if strings.ContainsAny(name, "/／\\、|｜,，;；\r\n\t") {
		return fmt.Errorf("信号分类概念组名称包含拼接符: %q", name)
	}
	return nil
}

// stabilizeSignalClassifications 移除低置信度别名并保留待重试状态。
// 输入：groups 是已经完整校验的模型分类结果。
// 输出：返回只包含高置信度别名的可持久化概念组。
// 副作用：无。
func stabilizeSignalClassifications(groups []signalGroupProposal) []signalGroupProposal {
	// 1. 每个概念组只保留达到自动归类阈值的成员。
	result := make([]signalGroupProposal, 0, len(groups))
	for _, group := range groups {
		highConfidence := make([]signalAliasProposal, 0, len(group.Aliases))
		for _, alias := range group.Aliases {
			if alias.Confidence >= signalClassificationConfidenceThreshold {
				highConfidence = append(highConfidence, alias)
			}
		}
		if len(highConfidence) > 0 {
			group.Aliases = highConfidence
			result = append(result, group)
		}
	}
	return result
}
