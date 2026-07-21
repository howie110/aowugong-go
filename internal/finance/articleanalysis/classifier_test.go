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
	content := "```json\n{\"groups\":[{\"canonical_name\":\"证券行业\",\"type\":\"sector\",\"aliases\":[{\"name\":\"券商\",\"confidence\":0.98}]}]}\n```"

	// 2. 不完整响应不得写入数据库形成半套映射。
	_, err := parseSignalClassificationJSON(content, candidates)
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
		content := `{"groups":[{"canonical_name":"` + name + `","type":"sector","aliases":[{"name":"券商","confidence":0.98}]}]}`
		_, err := parseSignalClassificationJSON(content, candidate)
		if err == nil || !strings.Contains(err.Error(), "概念组名称") {
			t.Fatalf("name %q error = %v, want canonical name validation", name, err)
		}
	}
}

// TestStabilizeSignalClassificationsLeavesLowConfidenceAliasUnmapped 验证低置信度分类保持待重试状态。
// 输入：模型把“券商”和低置信度“中信证券”同时归为证券行业。
// 输出：只持久化高置信度券商，中信证券不写入任何概念组。
// 副作用：无。
func TestStabilizeSignalClassificationsLeavesLowConfidenceAliasUnmapped(t *testing.T) {
	// 1. 构造包含高低两种置信度的模型分类。
	proposals := []signalGroupProposal{{
		CanonicalName: "证券行业", Type: "sector",
		Aliases: []signalAliasProposal{{Name: "券商", Confidence: 0.98}, {Name: "中信证券", Confidence: 0.60}},
	}}
	// 2. 低置信度名称必须保持未映射，允许下次使用更新词典重新分类。
	result := stabilizeSignalClassifications(proposals)
	if len(result) != 1 || result[0].CanonicalName != "证券行业" || len(result[0].Aliases) != 1 || result[0].Aliases[0].Name != "券商" {
		t.Fatalf("groups = %#v, want only high-confidence 券商", result)
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
	for _, fragment := range []string{"优先复用已有概念组", "具体公司上卷到最直接的行业或主题", "禁止使用斜杠拼接多个名称", "证券行业", "中信证券"} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt is missing %q", fragment)
		}
	}
}

// TestServiceClassifySignalAliasesPersistsUnknownNames 验证任务内部批量分类并持久化未知名称。
// 输入：六十天内的券商和中信证券信号、空概念词典及固定 DeepSeek 响应。
// 输出：新增两个别名并把两者写入同一证券行业组。
// 副作用：执行模拟 MySQL 查询、事务写入和模型调用。
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
	gateway := &fixedSignalClassificationGateway{response: `{"groups":[{"canonical_name":"证券行业","type":"sector","aliases":[{"name":"券商","confidence":0.98},{"name":"中信证券","confidence":0.96}]}]}`}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO investment_signal_group").
		WithArgs("证券行业", "sector", "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec("INSERT IGNORE INTO investment_signal_alias").
		WithArgs(int64(7), "券商", "券商", 0.98, "deepseek", "test-model").
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec("INSERT IGNORE INTO investment_signal_alias").
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
