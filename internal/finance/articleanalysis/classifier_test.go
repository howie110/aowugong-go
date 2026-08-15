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
	result, err := service.classifySignalBatch(context.Background(), nil, batch)
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

// TestParseSignalClassificationJSONUsesReuseCreateAndPendingActions 验证增量分类只接受复用、新建和待归类三态。
// 输入：分别复用现有证券行业、新建贵金属和暂缓处理含糊名称。
// 输出：两个高置信度映射进入待保存分组，含糊名称进入待归类列表。
// 副作用：无。
func TestParseSignalClassificationJSONUsesReuseCreateAndPendingActions(t *testing.T) {
	// 1. 构造三种决策及唯一可复用概念组。
	groups := []SignalGroup{
		{ID: 7, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}},
		{ID: 8, Name: pendingSignalGroupName, Type: pendingSignalGroupType, Aliases: []string{"里子"}},
	}
	candidates := []signalCandidate{{Name: "中信证券", Type: "stock"}, {Name: "黄金", Type: "commodity"}, {Name: "里子", Type: "other"}}
	content := `{"decisions":[` +
		`{"name":"中信证券","action":"reuse","existing_group_id":7,"confidence":0.96},` +
		`{"name":"黄金","action":"create","canonical_name":"贵金属","type":"commodity","confidence":0.94},` +
		`{"name":"里子","action":"pending","confidence":0.40}]}`

	// 2. 核对复用组名称由后端决定，新建组被保留，待归类不进入写库建议。
	result, err := parseSignalClassificationJSON(content, candidates, groups)
	if err != nil {
		t.Fatalf("parseSignalClassificationJSON() error = %v", err)
	}
	if len(result.Groups) != 2 || result.Groups[0].CanonicalName != "证券行业" || result.Groups[1].CanonicalName != "贵金属" {
		t.Fatalf("groups = %#v", result.Groups)
	}
	if len(result.Pending) != 1 || result.Pending[0].Name != "里子" {
		t.Fatalf("pending = %#v", result.Pending)
	}
}

// TestParseSignalClassificationJSONNormalizesPendingGroupReuse 验证模型复用待归类组时自动转为 pending。
// 输入：现有待归类组和错误使用 reuse 动作的含糊信号。
// 输出：信号进入待归类列表，不生成正式概念组且不让整批任务失败。
// 副作用：无。
func TestParseSignalClassificationJSONNormalizesPendingGroupReuse(t *testing.T) {
	// 1. 构造线上出现过的模型响应：把含糊信号复用到特殊待归类组。
	groups := []SignalGroup{{ID: 8, Name: pendingSignalGroupName, Type: pendingSignalGroupType}}
	candidates := []signalCandidate{{Name: "A股伪科技股", Type: "concept"}}
	content := `{"decisions":[{"name":"A股伪科技股","action":"reuse","existing_group_id":8,"confidence":0.96}]}`

	// 2. 后端应按 pending 语义接收，而不是重试模型后中断同步任务。
	result, err := parseSignalClassificationJSON(content, candidates, groups)
	if err != nil {
		t.Fatalf("parseSignalClassificationJSON() error = %v", err)
	}
	if len(result.Groups) != 0 || len(result.Pending) != 1 || result.Pending[0].Name != "A股伪科技股" {
		t.Fatalf("result = %#v", result)
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

// TestSignalGroupsForPersistenceAddsSinglePendingGroup 验证未确定标的写入统一待归类组。
// 输入：一个正式概念组和两个待归类名称。
// 输出：保留正式组并追加一个包含两个别名的待归类组，置信度为零。
// 副作用：无。
func TestSignalGroupsForPersistenceAddsSinglePendingGroup(t *testing.T) {
	// 1. 构造正式分类和含重复空白的待归类名称。
	groups := []signalGroupProposal{{CanonicalName: "证券行业", Type: "sector", Aliases: []signalAliasProposal{{Name: "券商", Confidence: 0.98}}}}
	pending := []string{"里子", " 期指 ", "里子"}

	// 2. 待归类名称必须去重并进入唯一特殊组。
	result := signalGroupsForPersistence(groups, pending)
	if len(result) != 2 || result[1].CanonicalName != pendingSignalGroupName || result[1].Type != pendingSignalGroupType {
		t.Fatalf("groups = %#v", result)
	}
	if len(result[1].Aliases) != 2 || result[1].Aliases[0].Name != "里子" || result[1].Aliases[1].Name != "期指" {
		t.Fatalf("pending aliases = %#v", result[1].Aliases)
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
	for _, fragment := range []string{"reuse、create、pending", "只有确实没有对应组时才使用 create", "existing_group_id", "待归类不是可复用的业务概念组", "具体公司上卷到最直接的行业或主题", "禁止使用斜杠拼接多个名称", "证券行业", "中信证券"} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt is missing %q", fragment)
		}
	}
}

// TestServiceClassifySignalAliasesPersistsUnknownNames 验证任务内部批量分类并持久化未知名称。
// 输入：六十天内的券商和中信证券信号、空概念词典及固定 DeepSeek 响应。
// 输出：新增两个别名并把两者写入同一证券行业组。
// 副作用：执行模拟 SQLite 查询、事务写入和模型调用。
func TestServiceClassifySignalAliasesPersistsUnknownNames(t *testing.T) {
	// 1. 准备统计行、空概念词典和完整分类响应。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COALESCE\\(analysis.recommendations_json").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recommendations_json", "risks_json", "market_mood", "prediction", "occurred_at"}).
			AddRow(`[{"name":"券商","type":"sector"},{"name":"中信证券","type":"stock"}]`, `[]`, "neutral", "range", "2026-07-20 10:00:00"))
	mock.ExpectQuery("SELECT g.id, g.canonical_name, g.group_type, a.alias_name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "canonical_name", "group_type", "alias_name"}))
	gateway := &fixedSignalClassificationGateway{response: `{"decisions":[{"name":"券商","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.98},{"name":"中信证券","action":"create","canonical_name":"证券行业","type":"sector","confidence":0.96}]}`}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO investment_signal_group").
		WithArgs("证券行业", "sector", "deepseek", "test-model", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "券商", "券商", 0.98, "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(7), "中信证券", "中信证券", 0.96, "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit()

	// 2. 执行分类并核对新增数量、提示词和数据库交互。
	service := NewService(NewRepository(db), ServiceOptions{Model: "test-model", Analyzer: gateway})
	inserted, err := service.classifySignalAliases(context.Background(), 60)
	if err != nil {
		t.Fatalf("classifySignalAliases() error = %v", err)
	}
	if inserted != 2 || !strings.Contains(gateway.prompt, "中信证券") {
		t.Fatalf("inserted = %d, prompt = %q", inserted, gateway.prompt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}
