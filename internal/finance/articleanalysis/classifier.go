package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const signalClassificationConfidenceThreshold = 0.80

const signalNewGroupConfidenceThreshold = 0.90

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

type signalClassificationDecision struct {
	Name            string  `json:"name"`
	Action          string  `json:"action"`
	ExistingGroupID int64   `json:"existing_group_id,omitempty"`
	CanonicalName   string  `json:"canonical_name,omitempty"`
	Type            string  `json:"type,omitempty"`
	Confidence      float64 `json:"confidence"`
}

type signalGroupProposal struct {
	CanonicalName string                `json:"canonical_name"`
	Type          string                `json:"type"`
	Aliases       []signalAliasProposal `json:"aliases"`
}

type signalClassificationPayload struct {
	Decisions []signalClassificationDecision `json:"decisions"`
}

type signalClassificationBatchResult struct {
	Groups  []signalGroupProposal
	Pending []signalCandidate
}

// classifySignalAliases 使用当前文章分析模型批量归类指定范围内的未知信号名称。
// 输入：ctx 控制数据库和模型调用，days 是至少一天的文章范围。
// 输出：返回本次新增别名数量；查询、分类或写入失败时返回错误。
// 副作用：调用当前分析模型，并写入 PostgreSQL 概念组和别名表。
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
	model, err := s.selectedAnalysisModel(ctx)
	if err != nil {
		return 0, err
	}
	if model.Analyzer == nil || !model.Analyzer.Configured() {
		return 0, fmt.Errorf("未配置可用的文章分析模型，仍有 %d 个投资信号等待分类", len(candidates))
	}

	// 2. 分批分类，每批写入后刷新词典供下一批优先复用。
	inserted := 0
	for start := 0; start < len(candidates); start += signalClassificationBatchSize {
		end := start + signalClassificationBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		classification, err := s.classifySignalBatch(ctx, groups, batch, model)
		if err != nil {
			return inserted, fmt.Errorf("处理第 %d 批投资信号分类: %w", start/signalClassificationBatchSize+1, err)
		}
		pendingNames := make([]string, 0, len(classification.Pending))
		for _, candidate := range classification.Pending {
			pendingNames = append(pendingNames, candidate.Name)
		}
		count, err := s.repository.SaveSignalGroups(ctx, signalGroupsForPersistence(classification.Groups, pendingNames), model.Model)
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

// classifySignalBatch 调用当前模型分类单批未知名称，并重试畸形响应。
// 输入：ctx 控制模型调用，groups 是现有词典，batch 是本批未知名称。
// 输出：返回只含高置信度别名的分类；连续失败时返回最后一次错误。
// 副作用：最多调用两次当前分析模型外部接口。
func (s *Service) classifySignalBatch(ctx context.Context, groups []SignalGroup, batch []signalCandidate, model analysisModelRuntime) (signalClassificationBatchResult, error) {
	// 1. 固定本批提示词，避免重试时改变统计语义。
	prompt := buildSignalClassificationPrompt(groups, batch)
	var lastErr error

	// 2. 模型调用或 JSON 校验失败时重试一次，成功后立即返回稳定结果。
	for attempt := 1; attempt <= signalClassificationMaxAttempts; attempt++ {
		content, err := model.Analyzer.SimpleChat(ctx, prompt, 4000)
		if err == nil {
			result, parseErr := parseSignalClassificationJSON(content, batch, groups)
			if parseErr == nil {
				return result, nil
			}
			err = parseErr
		}
		lastErr = err
		if ctxErr := ctx.Err(); ctxErr != nil {
			return signalClassificationBatchResult{}, fmt.Errorf("投资信号分类取消: %w", ctxErr)
		}
	}

	// 3. 保留最后一次业务上下文，便于任务失败通知定位具体原因。
	return signalClassificationBatchResult{}, fmt.Errorf("模型 %s 连续 %d 次返回无效分类: %w", model.Model, signalClassificationMaxAttempts, lastErr)
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
1. 每个名称必须明确选择 reuse、create、pending 三种 action 之一。
2. 优先使用 reuse，并通过 existing_group_id 引用已有概念组；只有确实没有对应组时才使用 create。
3. 待归类不是可复用的业务概念组；无法可靠判断时必须使用 pending action。
4. 具体公司上卷到最直接的行业或主题，例如证券公司归入“证券行业”。
5. create 的规范概念名使用简洁、通行的行业、主题、资产或市场名称，只能有一个名称，禁止使用斜杠拼接多个名称；type 只能使用 sector、concept、company、commodity、index、market、crypto 或 other。
6. 相关但统计含义不同的主题不要过度合并，选择与原始名称最直接的上级概念。
7. 无法可靠判断时使用 pending，不要为了完整而强行新建组。
8. 每个待分类名称必须且只能出现一次，name 必须原样返回。
9. confidence 是 0 到 1；reuse 至少 0.80，create 至少 0.90，否则使用 pending。

只返回以下结构的 JSON，不要解释，不要 Markdown：
{"decisions":[{"name":"中信证券","action":"reuse","existing_group_id":1,"confidence":0.96},{"name":"新主题","action":"create","canonical_name":"新主题行业","type":"sector","confidence":0.93},{"name":"含糊称呼","action":"pending","confidence":0.40}]}`,
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

// parseSignalClassificationJSON 解码并校验模型的增量归类决策。
// 输入：content 是模型文本，candidates 是本轮名称，groups 是允许复用的现有概念组。
// 输出：返回待保存概念组和待归类名称；遗漏、重复或越权决策会返回错误。
// 副作用：无。
func parseSignalClassificationJSON(content string, candidates []signalCandidate, groups []SignalGroup) (signalClassificationBatchResult, error) {
	// 1. 兼容纯 JSON 和 Markdown fenced JSON。
	content = strings.TrimSpace(content)
	if matches := fencedJSONPattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}
	var payload signalClassificationPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return signalClassificationBatchResult{}, fmt.Errorf("解析信号分类 JSON: %w", err)
	}
	return validateSignalClassificationPayload(payload, candidates, groups)
}

// validateSignalClassificationPayload 校验结构化分类决策并生成持久化建议。
// 输入：payload 是模型决策，candidates 是本轮名称，groups 是允许复用的现有概念组。
// 输出：返回待保存概念组和待归类名称；遗漏、重复或越权决策会返回错误。
// 副作用：无。
func validateSignalClassificationPayload(payload signalClassificationPayload, candidates []signalCandidate, groups []SignalGroup) (signalClassificationBatchResult, error) {
	// 1. 建立候选、已有组和已有规范名索引，后端决定 reuse 的最终组名。
	candidateIndex := make(map[string]signalCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateIndex[normalizeSignalAlias(candidate.Name)] = candidate
	}
	groupIndex := make(map[int64]SignalGroup, len(groups))
	existingNames := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		groupIndex[group.ID] = group
		existingNames[normalizeSignalAlias(group.Name)] = struct{}{}
	}

	// 2. 逐项验证三态动作，并把高置信度映射合并为仓储写入建议。
	seen := make(map[string]struct{}, len(candidates))
	result := signalClassificationBatchResult{Groups: []signalGroupProposal{}, Pending: []signalCandidate{}}
	positions := make(map[string]int)
	for _, decision := range payload.Decisions {
		key := normalizeSignalAlias(decision.Name)
		candidate, exists := candidateIndex[key]
		if !exists {
			return signalClassificationBatchResult{}, fmt.Errorf("信号分类返回未请求名称 %q", decision.Name)
		}
		if _, exists := seen[key]; exists {
			return signalClassificationBatchResult{}, fmt.Errorf("信号分类重复返回名称 %q", candidate.Name)
		}
		if decision.Confidence < 0 || decision.Confidence > 1 {
			return signalClassificationBatchResult{}, fmt.Errorf("信号分类 %q 置信度超出 0 到 1", candidate.Name)
		}
		seen[key] = struct{}{}

		var targetName, targetType string
		threshold := signalClassificationConfidenceThreshold
		switch strings.ToLower(strings.TrimSpace(decision.Action)) {
		case "reuse":
			group, exists := groupIndex[decision.ExistingGroupID]
			if !exists {
				return signalClassificationBatchResult{}, fmt.Errorf("信号分类 %q 复用了不存在的概念组 %d", candidate.Name, decision.ExistingGroupID)
			}
			if group.Name == pendingSignalGroupName || group.Type == pendingSignalGroupType {
				result.Pending = append(result.Pending, candidate)
				continue
			}
			targetName, targetType = group.Name, group.Type
		case "create":
			targetName, targetType = strings.TrimSpace(decision.CanonicalName), strings.TrimSpace(decision.Type)
			var err error
			targetType, err = validateSignalGroupDefinition(targetName, targetType)
			if err != nil {
				return signalClassificationBatchResult{}, err
			}
			if _, exists := existingNames[normalizeSignalAlias(targetName)]; exists {
				return signalClassificationBatchResult{}, fmt.Errorf("信号分类 %q 应复用已有概念组 %q，不得重复新建", candidate.Name, targetName)
			}
			threshold = signalNewGroupConfidenceThreshold
		case "pending":
			result.Pending = append(result.Pending, candidate)
			continue
		default:
			return signalClassificationBatchResult{}, fmt.Errorf("信号分类 %q action 无效: %q", candidate.Name, decision.Action)
		}
		if decision.Confidence < threshold {
			result.Pending = append(result.Pending, candidate)
			continue
		}

		proposalKey := normalizeSignalAlias(targetName)
		position, exists := positions[proposalKey]
		if !exists {
			position = len(result.Groups)
			positions[proposalKey] = position
			result.Groups = append(result.Groups, signalGroupProposal{CanonicalName: targetName, Type: targetType, Aliases: []signalAliasProposal{}})
		} else if result.Groups[position].Type != targetType {
			return signalClassificationBatchResult{}, fmt.Errorf("信号分类概念组 %q 类型冲突: %s/%s", targetName, result.Groups[position].Type, targetType)
		}
		result.Groups[position].Aliases = append(result.Groups[position].Aliases, signalAliasProposal{Name: candidate.Name, Confidence: decision.Confidence})
	}

	// 3. 缺少任一候选都拒绝整批结果，避免写入半套分类。
	missing := make([]string, 0)
	for _, candidate := range candidates {
		if _, exists := seen[normalizeSignalAlias(candidate.Name)]; !exists {
			missing = append(missing, candidate.Name)
		}
	}
	if len(missing) > 0 {
		return signalClassificationBatchResult{}, fmt.Errorf("信号分类缺少名称: %s", strings.Join(missing, "、"))
	}
	return result, nil
}

// signalGroupsForPersistence 给已确认概念组追加唯一的待归类别名组。
// 输入：groups 是模型确认映射，pendingNames 是暂时不能可靠判断的原始名称。
// 输出：返回可直接写入仓储的组列表；待归类名称去重且置信度为零。
// 副作用：无，不修改输入切片。
func signalGroupsForPersistence(groups []signalGroupProposal, pendingNames []string) []signalGroupProposal {
	// 1. 复制正式组并建立所有已归属别名索引。
	result := make([]signalGroupProposal, 0, len(groups)+1)
	for _, group := range groups {
		group.Aliases = append([]signalAliasProposal(nil), group.Aliases...)
		result = append(result, group)
	}
	seen := make(map[string]struct{})
	pendingPosition := -1
	for index, group := range result {
		if group.CanonicalName == pendingSignalGroupName || group.Type == pendingSignalGroupType {
			pendingPosition = index
		}
		for _, alias := range group.Aliases {
			key := normalizeSignalAlias(alias.Name)
			if key != "" {
				seen[key] = struct{}{}
			}
		}
	}

	// 2. 只在存在新名称时创建特殊组，并保持首次出现顺序。
	pendingAliases := make([]signalAliasProposal, 0, len(pendingNames))
	for _, value := range pendingNames {
		name := strings.TrimSpace(value)
		key := normalizeSignalAlias(name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pendingAliases = append(pendingAliases, signalAliasProposal{Name: name, Confidence: 0})
	}
	if len(pendingAliases) == 0 {
		return result
	}
	if pendingPosition >= 0 {
		result[pendingPosition].Aliases = append(result[pendingPosition].Aliases, pendingAliases...)
		return result
	}
	return append(result, signalGroupProposal{
		CanonicalName: pendingSignalGroupName, Type: pendingSignalGroupType, Aliases: pendingAliases,
	})
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

// validateSignalGroupDefinition 校验正式概念组名称和类型并返回规范类型。
// 输入：name 是规范名称，kind 是模型返回的组类型。
// 输出：名称与类型有效时返回小写规范类型，否则返回错误。
// 副作用：无。
func validateSignalGroupDefinition(name string, kind string) (string, error) {
	// 1. 先复用单一名称校验，再拒绝特殊待归类语义进入正式组。
	if err := validateSignalGroupName(name); err != nil {
		return "", err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if name == pendingSignalGroupName || kind == pendingSignalGroupType {
		return "", fmt.Errorf("信号分类正式概念组不得使用待归类名称或类型")
	}
	if kind == "" {
		kind = "other"
	}

	// 2. 只允许统计和页面已经约定的稳定类型枚举。
	allowed := map[string]struct{}{
		"sector": {}, "concept": {}, "company": {}, "commodity": {},
		"index": {}, "market": {}, "crypto": {}, "other": {},
	}
	if _, exists := allowed[kind]; !exists {
		return "", fmt.Errorf("信号分类概念组 %q 类型无效: %q", name, kind)
	}
	return kind, nil
}
