package articleanalysis

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type fixedSignalClassificationGateway struct {
	response string
	prompt   string
}

type sequenceSignalClassificationGateway struct {
	responses []string
	calls     int
}

// Configured 表示信号分类测试模型已配置。
// 输入：无。
// 输出：固定返回 true。
// 副作用：无。
func (fixedSignalClassificationGateway) Configured() bool {
	// 1. 允许服务进入 DeepSeek 分类阶段。
	return true
}

// SimpleChat 返回固定信号分类 JSON 并记录提示词。
// 输入：ctx、prompt 和 maxTokens 模拟正式模型调用。
// 输出：返回测试配置的响应文本。
// 副作用：记录最近一次提示词。
func (g *fixedSignalClassificationGateway) SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// 1. 保存提示词供断言并返回固定响应。
	g.prompt = prompt
	return g.response, nil
}

// Configured 表示顺序响应测试模型已配置。
// 输入：无。
// 输出：固定返回 true。
// 副作用：无。
func (sequenceSignalClassificationGateway) Configured() bool {
	// 1. 允许服务进入模型重试流程。
	return true
}

// SimpleChat 按调用顺序返回测试响应。
// 输入：ctx、prompt 和 maxTokens 模拟正式模型调用。
// 输出：依次返回预设文本，超过数量后继续返回最后一项。
// 副作用：累加模型调用次数。
func (g *sequenceSignalClassificationGateway) SimpleChat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// 1. 记录调用次数并选择当前响应。
	index := g.calls
	g.calls++
	if index >= len(g.responses) {
		index = len(g.responses) - 1
	}
	return g.responses[index], nil
}

// TestClassifySignalBatchRetriesMalformedJSON 验证模型返回截断 JSON 时自动重试当前批次。
// 输入：第一次为不完整 JSON，第二次为完整证券行业分类。
// 输出：成功返回分类结果且模型恰好调用两次。
// 副作用：调用顺序响应测试模型并累加次数。
func TestClassifySignalBatchRetriesMalformedJSON(t *testing.T) {
	// 1. 准备一次截断响应和一次可持久化响应。
	gateway := &sequenceSignalClassificationGateway{responses: []string{
		`{"decisions":[`,
		`{"decisions":[{"name":"券商","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.98}]}`,
	}}
	service := NewService(nil, ServiceOptions{Analyzer: gateway})
	batch := []signalCandidate{{Name: "券商", Type: "sector"}}

	// 2. 当前批次应在第二次响应后成功，而不是让整个任务直接失败。
	model := service.analysisModels[service.analysisModelOrder[0]]
	result, err := service.classifySignalBatch(context.Background(), nil, batch, model)
	if err != nil {
		t.Fatalf("classifySignalBatch() error = %v", err)
	}
	if gateway.calls != 2 || len(result.Groups) != 1 || len(result.Groups[0].Aliases) != 1 || result.Groups[0].Aliases[0].Name != "券商" {
		t.Fatalf("calls = %d, result = %#v", gateway.calls, result)
	}
}

// TestCollectUnknownSignalCandidatesSkipsMappedAliases 验证分类仅处理尚未映射的原始名称。
// 输入：已映射“券商”，文章同时包含“券商”和“中信证券”。
// 输出：只返回一次“中信证券”，并保留其原始类型。
// 副作用：无。
func TestCollectUnknownSignalCandidatesSkipsMappedAliases(t *testing.T) {
	// 1. 构造已有概念组和跨推荐、风险重复出现的未知名称。
	groups := []SignalGroup{{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}}}
	rows := []analysisRow{
		{Recommendations: []Signal{{Name: "券商", Type: "sector"}, {Name: "中信证券", Type: "stock"}}},
		{Risks: []Signal{{Name: "中信证券", Type: "stock"}}},
	}

	// 2. 已映射名称和重复未知名称都不能进入待分类列表。
	result := collectUnknownSignalCandidates(rows, groups)
	if len(result) != 1 || result[0].Name != "中信证券" || result[0].Type != "stock" {
		t.Fatalf("candidates = %#v, want one 中信证券 stock", result)
	}
}

// TestParseSignalClassificationJSONRequiresEveryCandidate 验证模型分类结果完整覆盖请求名称。
// 输入：两个未知名称和只返回其中一个名称的 fenced JSON。
// 输出：返回包含缺失名称的校验错误。
// 副作用：无。
func TestParseSignalClassificationJSONRequiresEveryCandidate(t *testing.T) {
	// 1. 模拟模型遗漏“中信证券”的不完整响应。
	candidates := []signalCandidate{{Name: "券商", Type: "sector"}, {Name: "中信证券", Type: "stock"}}
	content := "```json\n{\"decisions\":[{\"name\":\"券商\",\"action\":\"create\",\"canonical_name\":\"证券行业\",\"type\":\"sector\",\"confidence\":0.98}]}\n```"

	// 2. 不完整响应不得写入数据库形成半套映射。
	_, err := parseSignalClassificationJSON(content, candidates, nil)
	if err == nil || !strings.Contains(err.Error(), "中信证券") {
		t.Fatalf("error = %v, want missing 中信证券", err)
	}
}

// TestParseSignalClassificationJSONRejectsInvalidCanonicalName 验证规范名不能拼接别名或无限增长。
// 输入：包含斜杠的组合名称和超过二十字符的名称。
// 输出：两种响应都返回规范名校验错误。
// 副作用：无。
func TestParseSignalClassificationJSONRejectsInvalidCanonicalName(t *testing.T) {
	// 1. 为两个非法名称构造完整但不符合命名规则的模型响应。
	candidate := []signalCandidate{{Name: "券商", Type: "sector"}}
	invalidNames := []string{"券商/券商板块", strings.Repeat("超", 21)}

	// 2. 非法规范名必须在进入仓储前被拒绝。
	for _, name := range invalidNames {
		content := `{"decisions":[{"name":"券商","action":"create","canonical_name":"` + name + `","type":"sector","confidence":0.98}]}`
		_, err := parseSignalClassificationJSON(content, candidate, nil)
		if err == nil || !strings.Contains(err.Error(), "概念组名称") {
			t.Fatalf("name %q error = %v, want canonical name validation", name, err)
		}
	}
}

// TestParseSignalClassificationJSONUsesReuseAndCreateActions 验证增量分类只接受复用和新建两态。
// 输入：分别复用现有证券行业并新建贵金属概念组。
// 输出：两个映射都进入待保存分组，不产生模糊状态。
// 副作用：无。
func TestParseSignalClassificationJSONUsesReuseAndCreateActions(t *testing.T) {
	// 1. 构造两种明确决策及唯一可复用概念组。
	groups := []SignalGroup{{ID: 7, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}}}
	candidates := []signalCandidate{{Name: "中信证券", Type: "stock"}, {Name: "黄金", Type: "commodity"}}
	content := `{"decisions":[` +
		`{"name":"中信证券","action":"reuse","existing_group_id":7,"confidence":0.96},` +
		`{"name":"黄金","action":"create","canonical_name":"贵金属","type":"commodity","confidence":0.40}]}`

	// 2. 核对复用组名称由后端决定，低置信度新建组仍被明确保存。
	result, err := parseSignalClassificationJSON(content, candidates, groups)
	if err != nil {
		t.Fatalf("parseSignalClassificationJSON() error = %v", err)
	}
	if len(result.Groups) != 2 || result.Groups[0].CanonicalName != "证券行业" || result.Groups[1].CanonicalName != "贵金属" {
		t.Fatalf("groups = %#v", result.Groups)
	}
}

// TestParseSignalClassificationJSONConvertsPendingState 验证任何待归类输出都会转成明确兜底组。
// 输入：复用待归类组以及显式 pending 的两种模型响应。
// 输出：两种响应都生成“信息不明确”正式概念组，不能形成 pending 状态。
// 副作用：无。
func TestParseSignalClassificationJSONConvertsPendingState(t *testing.T) {
	// 1. 构造历史线上曾出现的两种模糊响应。
	groups := []SignalGroup{{ID: 8, Name: pendingSignalGroupName, Type: pendingSignalGroupType}}
	candidates := []signalCandidate{{Name: "A股伪科技股", Type: "concept"}}
	contents := []string{
		`{"decisions":[{"name":"A股伪科技股","action":"reuse","existing_group_id":8,"confidence":0.96}]}`,
		`{"decisions":[{"name":"A股伪科技股","action":"pending","confidence":0.40}]}`,
	}
	for _, content := range contents {
		result, err := parseSignalClassificationJSON(content, candidates, groups)
		if err != nil || len(result.Groups) != 1 || result.Groups[0].CanonicalName != unclearSignalGroupName || result.Groups[0].Aliases[0].Name != "A股伪科技股" {
			t.Fatalf("content = %s, result = %#v, error = %v", content, result, err)
		}
	}
}

// TestParseSignalClassificationJSONRejectsFakeReuseAndDuplicateCreate 验证模型不能伪造复用或用新建动作复制已有组。
// 输入：不存在的组 ID，以及与现有规范名相同的新建决策。
// 输出：两种响应都在写库前返回错误。
// 副作用：无。
func TestParseSignalClassificationJSONRejectsFakeReuseAndDuplicateCreate(t *testing.T) {
	// 1. 准备一个现有证券行业和一个待分类名称。
	groups := []SignalGroup{{ID: 7, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}}}
	candidates := []signalCandidate{{Name: "中信证券", Type: "stock"}}
	contents := []string{
		`{"decisions":[{"name":"中信证券","action":"reuse","existing_group_id":99,"confidence":0.96}]}`,
		`{"decisions":[{"name":"中信证券","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.96}]}`,
		`{"decisions":[{"name":"中信证券","action":"reuse","existing_group_id":8,"confidence":0.96}]}`,
		`{"decisions":[{"name":"中信证券","action":"create","canonical_name":"待归类","type":"sector","confidence":0.96}]}`,
		`{"decisions":[{"name":"中信证券","action":"create","canonical_name":"科技行业","type":"pending","confidence":0.96}]}`,
		`{"decisions":[{"name":"中信证券","action":"create","canonical_name":"科技行业","type":"mystery","confidence":0.96}]}`,
	}

	// 2. 非法复用和重复新建都不能生成持久化建议。
	for _, content := range contents {
		if _, err := parseSignalClassificationJSON(content, candidates, groups); err == nil {
			t.Fatalf("content %s unexpectedly accepted", content)
		}
	}
}

// TestAppendUniqueSignalCandidatesIncludesHistoricalPendingAliases 验证历史待归类别名重新进入明确分类流程。
// 输入：当前候选和包含重复名称的历史待归类别名。
// 输出：保留现有类型，并只追加一次历史名称。
// 副作用：无。
func TestAppendUniqueSignalCandidatesIncludesHistoricalPendingAliases(t *testing.T) {
	result := appendUniqueSignalCandidates(
		[]signalCandidate{{Name: "券商", Type: "sector"}}, []string{"券商", " 里子 ", "里子"},
	)
	if len(result) != 2 || result[0].Type != "sector" || result[1].Name != "里子" || result[1].Type != "other" {
		t.Fatalf("candidates = %#v", result)
	}
}

// TestFilterSignalClassificationDecisionsOnlyDropsNormalizedSignals 验证后端过滤信号与模型越权名称得到不同处理。
func TestFilterSignalClassificationDecisionsOnlyDropsNormalizedSignals(t *testing.T) {
	// 1. “科技大涨”会被结果导向规则删除，“凭空标的”从未出现在文章分析信号中。
	raw := AnalysisResult{Recommendations: []Signal{
		{Name: "贵州茅台（A股）", Type: "stock", Reason: "估值有望修复"},
		{Name: "科技大涨", Type: "sector", Reason: "今日领涨"},
	}}
	normalized := NormalizeAnalysis(raw)
	decisions := []signalClassificationDecision{
		{Name: "贵州茅台（A股）", Action: "create", CanonicalName: "白酒行业", Type: "sector", Confidence: 0.4},
		{Name: "科技大涨", Action: "create", CanonicalName: "科技行业", Type: "sector", Confidence: 0.4},
		{Name: "凭空标的", Action: "create", CanonicalName: "信息不明确", Type: "other", Confidence: 0.4},
	}

	// 2. 有效名称跟随后端压缩，已过滤信号消失，越权名称留下并交给严格校验拒绝。
	filtered := filterSignalClassificationDecisions(decisions, raw, analysisSignalCandidates(normalized))
	if len(filtered) != 2 || filtered[0].Name != "贵州茅台" || filtered[1].Name != "凭空标的" {
		t.Fatalf("filtered = %#v", filtered)
	}
	_, err := validateSignalClassificationPayload(
		signalClassificationPayload{Decisions: filtered}, analysisSignalCandidates(normalized), nil,
	)
	if err == nil || !strings.Contains(err.Error(), "未请求名称") {
		t.Fatalf("error = %v, want unknown-name rejection", err)
	}
}

// TestBuildSignalClassificationPromptRequiresCanonicalIndustryRollup 验证分类提示词使用稳定规范名并允许个股上卷。
// 输入：已有证券行业组和未知个股中信证券。
// 输出：提示词要求复用已有组、个股上卷且规范名禁止斜杠拼接。
// 副作用：无。
func TestBuildSignalClassificationPromptRequiresCanonicalIndustryRollup(t *testing.T) {
	// 1. 构造当前概念词典和待分类个股。
	groups := []SignalGroup{{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商", "券商板块"}}}
	candidates := []signalCandidate{{Name: "中信证券", Type: "stock"}}

	// 2. 核对决定最终统计口径的三项规则都进入模型提示词。
	prompt := buildSignalClassificationPrompt(groups, candidates)
	for _, fragment := range []string{"reuse、create 两种", "只有确实没有对应组时才使用 create", "existing_group_id", "禁止返回 pending", "信息不明确", "具体公司上卷到最直接的行业或主题", "禁止使用斜杠拼接多个名称", "证券行业", "中信证券"} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt is missing %q", fragment)
		}
	}
}

// TestServiceAnalyzeAndClassifyPendingPersistsUnknownNames 验证页面分析操作会补齐未知信号映射。
// 输入：没有待分析文章、六十天内的券商和中信证券信号、空概念词典及固定模型响应。
// 输出：即使未分析新文章，也新增两个别名并把两者写入同一证券行业组。
// 副作用：执行模拟 SQLite 查询、事务写入和模型调用。
func TestServiceAnalyzeAndClassifyPendingPersistsUnknownNames(t *testing.T) {
	// 1. 准备空待分析列表、统计行、空概念词典和完整分类响应。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT article.id, article.title").
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "link", "summary", "content", "published_at", "source_name", "source_type"}))
	mock.ExpectQuery("SELECT COALESCE\\(analysis.recommendations_json").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recommendations_json", "risks_json", "market_mood", "prediction", "occurred_at"}).
			AddRow(`[{"name":"券商","type":"sector"},{"name":"中信证券","type":"stock"}]`, `[]`, "neutral", "range", "2026-07-20 10:00:00"))
	mock.ExpectQuery("SELECT g.id, g.canonical_name, g.group_type, a.alias_name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "canonical_name", "group_type", "alias_name"}))
	mock.ExpectQuery("SELECT a.alias_name").
		WithArgs(pendingSignalGroupName, pendingSignalGroupType).
		WillReturnRows(sqlmock.NewRows([]string{"alias_name"}))
	gateway := &fixedSignalClassificationGateway{response: `{"decisions":[{"name":"券商","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.98},{"name":"中信证券","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.96}]}`}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO investment_signal_group").
		WithArgs("证券行业", "sector", "deepseek", "test-model", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "券商", "券商", 0.98, "deepseek", "test-model", pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "中信证券", "中信证券", 0.96, "deepseek", "test-model", pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec("DELETE FROM investment_signal_group").
		WithArgs(pendingSignalGroupName, pendingSignalGroupType).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	// 2. 执行分类并核对新增数量、提示词和数据库交互。
	service := NewService(NewRepository(db), ServiceOptions{Model: "test-model", Analyzer: gateway})
	result, err := service.AnalyzeAndClassifyPending(context.Background(), 10, false)
	if err != nil {
		t.Fatalf("AnalyzeAndClassifyPending() error = %v", err)
	}
	if result.AnalyzedCount != 0 || result.ClassifiedAliasCount != 2 || !strings.Contains(gateway.prompt, "中信证券") {
		t.Fatalf("result = %#v, prompt = %q", result, gateway.prompt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}
