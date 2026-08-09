package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	signalGlobalTargetMaxGroups   = 40
	signalRegroupingMaxTokens     = 8000
	signalSingletonCheckMinGroups = 20
)

type signalGroupSource struct {
	ID      string   `json:"source_id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Aliases []string `json:"aliases"`
	Count   int      `json:"count"`
}

type signalRegroupingSourceRef struct {
	SourceID   string  `json:"source_id"`
	Confidence float64 `json:"confidence"`
}

type signalRegroupingGroup struct {
	CanonicalName string                      `json:"canonical_name"`
	Type          string                      `json:"type"`
	Sources       []signalRegroupingSourceRef `json:"sources"`
}

type signalRegroupingPending struct {
	SourceID   string  `json:"source_id"`
	Confidence float64 `json:"confidence"`
}

type signalRegroupingPayload struct {
	Groups  []signalRegroupingGroup   `json:"groups"`
	Pending []signalRegroupingPending `json:"pending"`
}

type signalRegroupingResult struct {
	Groups         []signalGroupProposal
	PendingAliases []string
}

// SignalGroupRebuildGroup 描述全局归类预演中的一个目标概念组。
// 输入：由全局归类服务填充规范名称、类型和别名数量。
// 输出：作为人工确认重组结果的只读摘要。
// 副作用：无。
type SignalGroupRebuildGroup struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	AliasCount int    `json:"alias_count"`
}

// SignalGroupRebuildResult 描述一次全局概念组重建的预演或应用结果。
// 输入：由服务根据模型结果和写库状态填充。
// 输出：返回来源、旧组、新组、别名和待归类数量。
// 副作用：无。
type SignalGroupRebuildResult struct {
	Applied            bool                      `json:"applied"`
	SourceCount        int                       `json:"source_count"`
	PreviousGroupCount int                       `json:"previous_group_count"`
	GroupCount         int                       `json:"group_count"`
	AliasCount         int                       `json:"alias_count"`
	PendingAliasCount  int                       `json:"pending_alias_count"`
	PendingAliases     []string                  `json:"pending_aliases"`
	Groups             []SignalGroupRebuildGroup `json:"groups"`
}

// RebuildSignalGroups 使用 DeepSeek 全局收敛现有概念组和未归类标的。
// 输入：ctx 控制查询和模型调用，days 是统计范围，apply 控制是否替换线上词典。
// 输出：返回可审查的重组摘要；模型结果不完整或不安全时返回错误。
// 副作用：调用 DeepSeek；apply 为 true 时在单个事务内重建 PostgreSQL 概念词典。
func (s *Service) RebuildSignalGroups(ctx context.Context, days int, apply bool) (SignalGroupRebuildResult, error) {
	// 1. 读取完整旧词典和统计范围内的原始信号，构造不丢失来源的全局输入。
	if days < 1 {
		days = DefaultTargetDays
	}
	rows, err := s.repository.analysisRows(ctx, days)
	if err != nil {
		return SignalGroupRebuildResult{}, fmt.Errorf("读取全局归类投资信号: %w", err)
	}
	groups, err := s.repository.SignalGroups(ctx)
	if err != nil {
		return SignalGroupRebuildResult{}, err
	}
	sources := buildSignalGroupSources(rows, groups)
	if len(sources) == 0 {
		return SignalGroupRebuildResult{PendingAliases: []string{}, Groups: []SignalGroupRebuildGroup{}}, nil
	}
	if s.options.Analyzer == nil || !s.options.Analyzer.Configured() {
		return SignalGroupRebuildResult{}, fmt.Errorf("未配置 DEEPSEEK_API_KEY，无法执行投资信号全局归类")
	}

	// 2. 重试畸形或过细响应，并由后端验证来源完整性和唯一归属。
	regrouped, err := s.regroupSignalSources(ctx, sources)
	if err != nil {
		return SignalGroupRebuildResult{}, err
	}
	if len(regrouped.Groups) == 0 {
		return SignalGroupRebuildResult{}, fmt.Errorf("投资信号全局归类没有生成可应用的概念组")
	}

	// 3. 先形成只读摘要；只有明确 apply 时才原子替换词典。
	result := summarizeSignalRegrouping(regrouped, len(sources), len(groups))
	if !apply {
		return result, nil
	}
	groupsForPersistence := signalGroupsForPersistence(regrouped.Groups, regrouped.PendingAliases)
	if _, err := s.repository.ReplaceSignalGroups(ctx, groupsForPersistence, s.options.Model); err != nil {
		return SignalGroupRebuildResult{}, err
	}
	result.Applied = true
	return result, nil
}

// regroupSignalSources 调用 DeepSeek 并重试畸形、不完整或仍然过细的全局结果。
// 输入：ctx 控制模型调用，sources 是必须唯一覆盖的全部来源。
// 输出：返回不超过四十组的完整建议；连续失败时返回最后一次业务错误。
// 副作用：最多调用两次 DeepSeek 外部接口。
func (s *Service) regroupSignalSources(ctx context.Context, sources []signalGroupSource) (signalRegroupingResult, error) {
	// 1. 固定基础输入，并在重试时附加上一次后端校验错误作为纠正要求。
	basePrompt := buildSignalRegroupingPrompt(sources)
	var lastErr error
	for attempt := 1; attempt <= signalClassificationMaxAttempts; attempt++ {
		prompt := basePrompt
		if lastErr != nil {
			prompt += fmt.Sprintf("\n\n上一次输出未通过后端校验：%s。请完整重新输出修正后的 JSON。", lastErr.Error())
		}
		content, err := s.options.Analyzer.SimpleChat(ctx, prompt, signalRegroupingMaxTokens)
		if err == nil {
			var result signalRegroupingResult
			result, err = parseSignalRegroupingJSON(content, sources)
			if err == nil {
				err = validateSignalRegroupingGranularity(result)
			}
			if err == nil {
				return result, nil
			}
		}
		lastErr = err
		if ctxErr := ctx.Err(); ctxErr != nil {
			return signalRegroupingResult{}, fmt.Errorf("投资信号全局归类取消: %w", ctxErr)
		}
	}

	// 2. 保留最终校验原因供任务失败通知定位。
	return signalRegroupingResult{}, fmt.Errorf("DeepSeek 连续 %d 次返回无效全局归类: %w", signalClassificationMaxAttempts, lastErr)
}

// buildSignalGroupSources 把现有概念组和未归类原始标的转换为全局归类来源。
// 输入：rows 是统计期信号，groups 是完整现有词典。
// 输出：返回稳定来源 ID、别名和出现次数；每个原始标的只出现一次。
// 副作用：无。
func buildSignalGroupSources(rows []analysisRow, groups []SignalGroup) []signalGroupSource {
	// 1. 统计每个原始标的在推荐和风险中的总出现次数。
	counts := make(map[string]int)
	for _, row := range rows {
		for _, signals := range [][]Signal{row.Recommendations, row.Risks} {
			for _, signal := range signals {
				key := normalizeSignalAlias(signal.Name)
				if key != "" {
					counts[key]++
				}
			}
		}
	}

	// 2. 每个旧组作为不可拆分来源，保留其全部原始别名并累计热度。
	individualIndex := 0
	result := make([]signalGroupSource, 0, len(groups))
	for _, group := range groups {
		if group.Name == pendingSignalGroupName || group.Type == pendingSignalGroupType {
			for _, alias := range uniqueSignalAliases(group.Aliases) {
				result = append(result, signalGroupSource{
					ID: fmt.Sprintf("alias:%d", individualIndex), Name: alias, Type: "other",
					Aliases: []string{alias}, Count: counts[normalizeSignalAlias(alias)],
				})
				individualIndex++
			}
			continue
		}
		aliases := uniqueSignalAliases(group.Aliases)
		count := 0
		for _, alias := range aliases {
			count += counts[normalizeSignalAlias(alias)]
		}
		result = append(result, signalGroupSource{
			ID: fmt.Sprintf("group:%d", group.ID), Name: group.Name, Type: group.Type, Aliases: aliases, Count: count,
		})
	}

	// 3. 未进入旧词典的名称各自成为来源，交给模型复用、合并或暂缓。
	unknown := collectUnknownSignalCandidates(rows, groups)
	for _, candidate := range unknown {
		result = append(result, signalGroupSource{
			ID: fmt.Sprintf("alias:%d", individualIndex), Name: candidate.Name, Type: candidate.Type,
			Aliases: []string{candidate.Name}, Count: counts[normalizeSignalAlias(candidate.Name)],
		})
		individualIndex++
	}
	return result
}

// buildSignalRegroupingPrompt 构造一次性全局概念收敛提示词。
// 输入：sources 是全部旧组和未知标的，来源 ID 在本轮内唯一。
// 输出：返回要求 20 到 40 个稳定概念组的严格 JSON 提示词。
// 副作用：无。
func buildSignalRegroupingPrompt(sources []signalGroupSource) string {
	// 1. 使用结构化 JSON 传递全部来源，避免模型遗漏旧词典内容。
	sourcesJSON, _ := json.Marshal(sources)

	// 2. 明确唯一归属、收敛尺度和待归类出口，避免再次生成大量单例组。
	return fmt.Sprintf(`你负责把投资文章信号词典做一次全局重组。输入来源包含已有概念组和尚未归类的原始标的：
%s

规则：
1. 从全局语义统一判断，尽量收敛为 20 到 40 个长期稳定、可统计的行业、主题、资产或市场概念组。
2. 每个来源必须且只能出现一次：放入某个 groups[].sources，或放入 pending；禁止遗漏、重复和虚构 source_id。
3. 一个原始标的只属于一个概念组。已有来源内部的 aliases 不得拆开，但语义相近的多个旧组应合并。
4. 公司、产品和细分称呼优先上卷到最直接且有统计意义的行业或主题，例如“中信证券”“券商板块”归入“证券行业”。
5. 不要为了凑数量创建单例组；只有真正独立且长期有统计意义的资产或主题才允许单独成组。
6. canonical_name 必须是一个简洁通行名称，禁止用斜杠或列表拼接；type 使用 sector、concept、company、commodity、index、market、crypto 或 other。
7. confidence 是 0 到 1；低于 0.80 或无法可靠判断的来源放入 pending。
8. source_id 必须原样返回。只返回 JSON，不要解释，不要 Markdown。

输出结构：
{"groups":[{"canonical_name":"证券行业","type":"sector","sources":[{"source_id":"group:1","confidence":0.98},{"source_id":"alias:0","confidence":0.95}]}],"pending":[{"source_id":"alias:1","confidence":0.40}]}`,
		string(sourcesJSON))
}

// parseSignalRegroupingJSON 解码并验证全局概念重组结果。
// 输入：content 是模型文本，sources 是本轮允许引用的全部来源。
// 输出：返回完整概念组建议和待归类别名；来源遗漏、重复或越权时返回错误。
// 副作用：无。
func parseSignalRegroupingJSON(content string, sources []signalGroupSource) (signalRegroupingResult, error) {
	// 1. 兼容纯 JSON 和 fenced JSON，并建立允许来源索引。
	content = strings.TrimSpace(content)
	if matches := fencedJSONPattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}
	var payload signalRegroupingPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return signalRegroupingResult{}, fmt.Errorf("解析信号全局归类 JSON: %w", err)
	}
	sourceIndex := make(map[string]signalGroupSource, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.ID) == "" {
			return signalRegroupingResult{}, fmt.Errorf("信号全局归类包含空来源 ID")
		}
		if _, exists := sourceIndex[source.ID]; exists {
			return signalRegroupingResult{}, fmt.Errorf("信号全局归类输入重复来源 %q", source.ID)
		}
		sourceIndex[source.ID] = source
	}

	// 2. 校验目标组并展开被合并来源的全部原始别名。
	result := signalRegroupingResult{Groups: []signalGroupProposal{}, PendingAliases: []string{}}
	seenSources := make(map[string]struct{}, len(sources))
	seenGroups := make(map[string]struct{})
	seenAliases := make(map[string]struct{})
	for _, group := range payload.Groups {
		name := strings.TrimSpace(group.CanonicalName)
		kind, err := validateSignalGroupDefinition(name, group.Type)
		if err != nil {
			return signalRegroupingResult{}, err
		}
		nameKey := normalizeSignalAlias(name)
		if _, exists := seenGroups[nameKey]; exists {
			return signalRegroupingResult{}, fmt.Errorf("信号全局归类重复概念组 %q", name)
		}
		seenGroups[nameKey] = struct{}{}
		proposal := signalGroupProposal{CanonicalName: name, Type: kind, Aliases: []signalAliasProposal{}}
		for _, reference := range group.Sources {
			source, err := consumeSignalRegroupingSource(reference.SourceID, reference.Confidence, sourceIndex, seenSources)
			if err != nil {
				return signalRegroupingResult{}, err
			}
			if reference.Confidence < signalClassificationConfidenceThreshold {
				appendPendingSignalAliases(&result, source.Aliases, seenAliases)
				continue
			}
			for _, alias := range source.Aliases {
				key := normalizeSignalAlias(alias)
				if key == "" {
					continue
				}
				if _, exists := seenAliases[key]; exists {
					return signalRegroupingResult{}, fmt.Errorf("信号全局归类别名重复归属 %q", alias)
				}
				seenAliases[key] = struct{}{}
				proposal.Aliases = append(proposal.Aliases, signalAliasProposal{Name: strings.TrimSpace(alias), Confidence: reference.Confidence})
			}
		}
		if len(proposal.Aliases) > 0 {
			result.Groups = append(result.Groups, proposal)
		}
	}

	// 3. 验证显式待归类来源，并确保模型完整覆盖每一个输入来源。
	for _, pending := range payload.Pending {
		source, err := consumeSignalRegroupingSource(pending.SourceID, pending.Confidence, sourceIndex, seenSources)
		if err != nil {
			return signalRegroupingResult{}, err
		}
		appendPendingSignalAliases(&result, source.Aliases, seenAliases)
	}
	missing := make([]string, 0)
	for _, source := range sources {
		if _, exists := seenSources[source.ID]; !exists {
			missing = append(missing, source.Name)
		}
	}
	if len(missing) > 0 {
		return signalRegroupingResult{}, fmt.Errorf("信号全局归类缺少来源: %s", strings.Join(missing, "、"))
	}
	return result, nil
}

// validateSignalRegroupingGranularity 拒绝组数超限或单别名组占主导的过细结果。
// 输入：result 是已经通过来源完整性校验的全局建议。
// 输出：粒度可接受时返回 nil，否则返回可用于模型纠正重试的错误。
// 副作用：无。
func validateSignalRegroupingGranularity(result signalRegroupingResult) error {
	// 1. 正式概念组始终不能超过全局硬上限。
	if len(result.Groups) > signalGlobalTargetMaxGroups {
		return fmt.Errorf("投资信号全局归类仍有 %d 组，超过 %d 组上限", len(result.Groups), signalGlobalTargetMaxGroups)
	}

	// 2. 规模达到目标区间后，过半单别名组说明没有完成有效语义收敛。
	if len(result.Groups) >= signalSingletonCheckMinGroups {
		singletons := 0
		for _, group := range result.Groups {
			if len(group.Aliases) == 1 {
				singletons++
			}
		}
		if singletons*2 > len(result.Groups) {
			return fmt.Errorf("投资信号全局归类单别名概念组过多: %d/%d", singletons, len(result.Groups))
		}
	}
	return nil
}

// consumeSignalRegroupingSource 校验并占用一个模型返回的来源引用。
// 输入：sourceID 和 confidence 来自模型，sourceIndex 与 seen 分别限定范围和唯一性。
// 输出：返回对应来源；越权、重复或置信度非法时返回错误。
// 副作用：把已验证来源写入 seen 集合。
func consumeSignalRegroupingSource(sourceID string, confidence float64, sourceIndex map[string]signalGroupSource, seen map[string]struct{}) (signalGroupSource, error) {
	// 1. 来源只能引用本轮输入，且全局只能出现一次。
	source, exists := sourceIndex[sourceID]
	if !exists {
		return signalGroupSource{}, fmt.Errorf("信号全局归类返回未请求来源 %q", sourceID)
	}
	if _, exists := seen[sourceID]; exists {
		return signalGroupSource{}, fmt.Errorf("信号全局归类重复返回来源 %q", source.Name)
	}
	if confidence < 0 || confidence > 1 {
		return signalGroupSource{}, fmt.Errorf("信号全局归类来源 %q 置信度超出 0 到 1", source.Name)
	}
	seen[sourceID] = struct{}{}
	return source, nil
}

// appendPendingSignalAliases 把来源别名追加到待归类集合并保持唯一顺序。
// 输入：result 是当前结果，aliases 是来源别名，seen 记录全局已归属名称。
// 输出：无；重复归属不会再次追加。
// 副作用：修改 result.PendingAliases 和 seen。
func appendPendingSignalAliases(result *signalRegroupingResult, aliases []string, seen map[string]struct{}) {
	// 1. 清理空值并按来源顺序记录唯一待归类别名。
	for _, alias := range aliases {
		name := strings.TrimSpace(alias)
		key := normalizeSignalAlias(name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result.PendingAliases = append(result.PendingAliases, name)
	}
}

// uniqueSignalAliases 清理并去重一个来源中的原始别名。
// 输入：aliases 是数据库稳定顺序的原始名称。
// 输出：返回去除空值和大小写重复后的名称数组。
// 副作用：无。
func uniqueSignalAliases(aliases []string) []string {
	// 1. 按首次出现顺序保留展示原文。
	seen := make(map[string]struct{}, len(aliases))
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		name := strings.TrimSpace(alias)
		key := normalizeSignalAlias(name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

// summarizeSignalRegrouping 把内部重组建议转换为稳定的人工审查摘要。
// 输入：regrouped 是已校验结果，sourceCount 和 previousGroupCount 描述变更规模。
// 输出：返回按别名数量倒序的概念组摘要和完整待归类名单。
// 副作用：无。
func summarizeSignalRegrouping(regrouped signalRegroupingResult, sourceCount int, previousGroupCount int) SignalGroupRebuildResult {
	// 1. 统计可写入别名数量并生成组级摘要。
	groups := make([]SignalGroupRebuildGroup, 0, len(regrouped.Groups))
	aliasCount := 0
	for _, group := range regrouped.Groups {
		aliasCount += len(group.Aliases)
		groups = append(groups, SignalGroupRebuildGroup{Name: group.CanonicalName, Type: group.Type, AliasCount: len(group.Aliases)})
	}
	sort.SliceStable(groups, func(left, right int) bool {
		if groups[left].AliasCount == groups[right].AliasCount {
			return groups[left].Name < groups[right].Name
		}
		return groups[left].AliasCount > groups[right].AliasCount
	})

	// 2. 待归类名称保持模型来源顺序，便于人工定位和后续增量处理。
	pending := append([]string(nil), regrouped.PendingAliases...)
	return SignalGroupRebuildResult{
		SourceCount: sourceCount, PreviousGroupCount: previousGroupCount,
		GroupCount: len(regrouped.Groups), AliasCount: aliasCount,
		PendingAliasCount: len(pending), PendingAliases: pending, Groups: groups,
	}
}
