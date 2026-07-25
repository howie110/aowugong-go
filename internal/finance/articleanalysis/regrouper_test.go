package articleanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type sequenceSignalRegroupingGateway struct {
	responses []string
	prompts   []string
}

// Configured 表示全局归类顺序响应模型已配置。
// 输入：无。
// 输出：固定返回 true。
// 副作用：无。
func (sequenceSignalRegroupingGateway) Configured() bool {
	// 1. 允许服务进入模型重试流程。
	return true
}

// SimpleChat 按调用顺序返回全局归类响应并记录提示词。
// 输入：ctx、prompt 和 maxTokens 模拟正式模型调用。
// 输出：依次返回预设文本。
// 副作用：保存每次提示词并消耗一个响应。
func (g *sequenceSignalRegroupingGateway) SimpleChat(_ context.Context, prompt string, _ int) (string, error) {
	// 1. 记录提示词并按当前调用位置返回响应。
	index := len(g.prompts)
	g.prompts = append(g.prompts, prompt)
	if index >= len(g.responses) {
		index = len(g.responses) - 1
	}
	return g.responses[index], nil
}

// TestParseSignalRegroupingJSONMergesSourcesAndExpandsAliases 验证全局归类把旧组和未归类来源合并成更宽的概念组。
// 输入：两个旧科技组和一个未归类原始标的，以及一份完整模型映射。
// 输出：生成一个科技行业组，展开全部三个原始别名，含糊来源保持待归类。
// 副作用：无。
func TestParseSignalRegroupingJSONMergesSourcesAndExpandsAliases(t *testing.T) {
	// 1. 准备可被全局合并的稳定来源集合。
	sources := []signalGroupSource{
		{ID: "group:1", Name: "人工智能", Type: "concept", Aliases: []string{"AI硬件", "AI应用"}, Count: 12},
		{ID: "group:2", Name: "科技行业", Type: "sector", Aliases: []string{"科技股"}, Count: 8},
		{ID: "alias:0", Name: "里子", Type: "other", Aliases: []string{"里子"}, Count: 2},
	}
	content := `{"groups":[{"canonical_name":"科技行业","type":"sector","sources":[` +
		`{"source_id":"group:1","confidence":0.95},{"source_id":"group:2","confidence":0.98}]}],` +
		`"pending":[{"source_id":"alias:0","confidence":0.35}]}`

	// 2. 旧组别名全部迁入新组，待归类来源不进入映射。
	result, err := parseSignalRegroupingJSON(content, sources)
	if err != nil {
		t.Fatalf("parseSignalRegroupingJSON() error = %v", err)
	}
	if len(result.Groups) != 1 || result.Groups[0].CanonicalName != "科技行业" {
		t.Fatalf("groups = %#v", result.Groups)
	}
	aliases := result.Groups[0].Aliases
	if len(aliases) != 3 || aliases[0].Name != "AI硬件" || aliases[1].Name != "AI应用" || aliases[2].Name != "科技股" {
		t.Fatalf("aliases = %#v", aliases)
	}
	if len(result.PendingAliases) != 1 || result.PendingAliases[0] != "里子" {
		t.Fatalf("pending = %#v", result.PendingAliases)
	}
}

// TestParseSignalRegroupingJSONRequiresEverySource 验证全局重建结果必须完整覆盖每个旧组和未知来源。
// 输入：两个来源但模型只返回其中一个。
// 输出：返回包含遗漏来源名称的错误。
// 副作用：无。
func TestParseSignalRegroupingJSONRequiresEverySource(t *testing.T) {
	// 1. 构造遗漏“贵金属”的不完整响应。
	sources := []signalGroupSource{
		{ID: "group:1", Name: "证券行业", Aliases: []string{"券商"}},
		{ID: "group:2", Name: "贵金属", Aliases: []string{"黄金"}},
	}
	content := `{"groups":[{"canonical_name":"金融行业","type":"sector","sources":[{"source_id":"group:1","confidence":0.95}]}],"pending":[]}`

	// 2. 不完整结果不得进入数据库替换阶段。
	_, err := parseSignalRegroupingJSON(content, sources)
	if err == nil || !strings.Contains(err.Error(), "贵金属") {
		t.Fatalf("error = %v, want missing 贵金属", err)
	}
}

// TestBuildSignalGroupSourcesSplitsPersistedPendingAliases 验证下次全局重组会逐个重审待归类别名。
// 输入：一个正式组、一个包含两个别名的待归类组和两条当前信号。
// 输出：正式组保持整体来源，待归类组拆成两个独立 alias 来源。
// 副作用：无。
func TestBuildSignalGroupSourcesSplitsPersistedPendingAliases(t *testing.T) {
	// 1. 构造正式证券组、持久化待归类组；其中一个旧别名不在当前窗口出现。
	groups := []SignalGroup{
		{ID: 1, Name: "证券行业", Type: "sector", Aliases: []string{"券商"}},
		{ID: 2, Name: pendingSignalGroupName, Type: pendingSignalGroupType, Aliases: []string{"里子", "期指", "旧称呼"}},
	}
	rows := []analysisRow{{Recommendations: []Signal{{Name: "券商"}, {Name: "里子"}, {Name: "期指"}}}}

	// 2. 特殊组不能成为不可拆分来源，窗口外别名也必须保留并分别供模型判断。
	sources := buildSignalGroupSources(rows, groups)
	if len(sources) != 4 || sources[0].ID != "group:1" || sources[1].ID != "alias:0" || sources[2].ID != "alias:1" || sources[3].ID != "alias:2" {
		t.Fatalf("sources = %#v", sources)
	}
	if sources[1].Name != "里子" || sources[2].Name != "期指" || sources[3].Name != "旧称呼" || sources[3].Count != 0 {
		t.Fatalf("pending sources = %#v", sources[1:])
	}
}

// TestParseSignalRegroupingJSONRejectsReservedOrUnknownGroupDefinitions 验证正式组不能伪装成待归类或使用未知类型。
// 输入：一个来源及三份保留名称、保留类型、未知类型响应。
// 输出：全部在生成替换建议前返回错误。
// 副作用：无。
func TestParseSignalRegroupingJSONRejectsReservedOrUnknownGroupDefinitions(t *testing.T) {
	// 1. 构造一个允许来源和三种非法正式组定义。
	sources := []signalGroupSource{{ID: "alias:0", Name: "券商", Aliases: []string{"券商"}}}
	contents := []string{
		`{"groups":[{"canonical_name":"待归类","type":"sector","sources":[{"source_id":"alias:0","confidence":0.95}]}],"pending":[]}`,
		`{"groups":[{"canonical_name":"证券行业","type":"pending","sources":[{"source_id":"alias:0","confidence":0.95}]}],"pending":[]}`,
		`{"groups":[{"canonical_name":"证券行业","type":"mystery","sources":[{"source_id":"alias:0","confidence":0.95}]}],"pending":[]}`,
	}

	// 2. 特殊语义和未知枚举不能进入正式概念词典。
	for _, content := range contents {
		if _, err := parseSignalRegroupingJSON(content, sources); err == nil {
			t.Fatalf("content %s unexpectedly accepted", content)
		}
	}
}

// TestRegroupSignalSourcesRetriesWhenResultStillHasTooManyGroups 验证过细的全局结果会携带错误重试。
// 输入：四十一个来源，模型先逐项建组，再把全部来源收敛成一个组。
// 输出：服务调用模型两次并接受第二次结果，纠正提示包含组数上限。
// 副作用：调用顺序响应测试模型两次。
func TestRegroupSignalSourcesRetriesWhenResultStillHasTooManyGroups(t *testing.T) {
	// 1. 构造四十一个来源及一份超过硬上限的首次响应。
	sources := make([]signalGroupSource, 0, signalGlobalTargetMaxGroups+1)
	tooMany := signalRegroupingPayload{Groups: []signalRegroupingGroup{}, Pending: []signalRegroupingPending{}}
	mergedRefs := make([]signalRegroupingSourceRef, 0, signalGlobalTargetMaxGroups+1)
	for index := 0; index <= signalGlobalTargetMaxGroups; index++ {
		id := fmt.Sprintf("alias:%d", index)
		name := fmt.Sprintf("标的%d", index)
		sources = append(sources, signalGroupSource{ID: id, Name: name, Aliases: []string{name}})
		reference := signalRegroupingSourceRef{SourceID: id, Confidence: 0.95}
		tooMany.Groups = append(tooMany.Groups, signalRegroupingGroup{CanonicalName: fmt.Sprintf("概念%d", index), Type: "concept", Sources: []signalRegroupingSourceRef{reference}})
		mergedRefs = append(mergedRefs, reference)
	}
	tooManyJSON, _ := json.Marshal(tooMany)
	mergedJSON, _ := json.Marshal(signalRegroupingPayload{Groups: []signalRegroupingGroup{{CanonicalName: "综合概念", Type: "concept", Sources: mergedRefs}}, Pending: []signalRegroupingPending{}})
	gateway := &sequenceSignalRegroupingGateway{responses: []string{string(tooManyJSON), string(mergedJSON)}}

	// 2. 首次组数超限必须触发带原因的第二次请求。
	service := NewService(nil, ServiceOptions{Analyzer: gateway})
	result, err := service.regroupSignalSources(context.Background(), sources)
	if err != nil {
		t.Fatalf("regroupSignalSources() error = %v", err)
	}
	if len(result.Groups) != 1 || len(gateway.prompts) != 2 {
		t.Fatalf("groups = %d, prompts = %d", len(result.Groups), len(gateway.prompts))
	}
	if !strings.Contains(gateway.prompts[1], "超过 40 组上限") {
		t.Fatalf("retry prompt = %q", gateway.prompts[1])
	}
}

// TestRegroupSignalSourcesRetriesWhenSingletonGroupsDominate 验证大量单别名组会被判定为仍然过细。
// 输入：三十个来源，模型先逐项建组，再合并成一个概念组。
// 输出：服务调用模型两次，纠正提示包含单别名组过多原因。
// 副作用：调用顺序响应测试模型两次。
func TestRegroupSignalSourcesRetriesWhenSingletonGroupsDominate(t *testing.T) {
	// 1. 构造三十个来源和三十个单别名组。
	sources := make([]signalGroupSource, 0, 30)
	overfine := signalRegroupingPayload{Groups: []signalRegroupingGroup{}, Pending: []signalRegroupingPending{}}
	mergedRefs := make([]signalRegroupingSourceRef, 0, 30)
	for index := 0; index < 30; index++ {
		id := fmt.Sprintf("alias:%d", index)
		name := fmt.Sprintf("相近标的%d", index)
		sources = append(sources, signalGroupSource{ID: id, Name: name, Aliases: []string{name}})
		reference := signalRegroupingSourceRef{SourceID: id, Confidence: 0.95}
		overfine.Groups = append(overfine.Groups, signalRegroupingGroup{CanonicalName: fmt.Sprintf("细分概念%d", index), Type: "concept", Sources: []signalRegroupingSourceRef{reference}})
		mergedRefs = append(mergedRefs, reference)
	}
	overfineJSON, _ := json.Marshal(overfine)
	mergedJSON, _ := json.Marshal(signalRegroupingPayload{Groups: []signalRegroupingGroup{{CanonicalName: "综合概念", Type: "concept", Sources: mergedRefs}}, Pending: []signalRegroupingPending{}})
	gateway := &sequenceSignalRegroupingGateway{responses: []string{string(overfineJSON), string(mergedJSON)}}

	// 2. 组数虽低于四十，但单别名占比过高仍必须重试。
	service := NewService(nil, ServiceOptions{Analyzer: gateway})
	result, err := service.regroupSignalSources(context.Background(), sources)
	if err != nil {
		t.Fatalf("regroupSignalSources() error = %v", err)
	}
	if len(result.Groups) != 1 || len(gateway.prompts) != 2 {
		t.Fatalf("groups = %d, prompts = %d", len(result.Groups), len(gateway.prompts))
	}
	if !strings.Contains(gateway.prompts[1], "单别名概念组") {
		t.Fatalf("retry prompt = %q", gateway.prompts[1])
	}
}

// TestServiceRebuildSignalGroupsDryRunDoesNotWriteDatabase 验证全局归类支持先只读预演。
// 输入：一个已有证券组、一个未知黄金标的和固定的全局归类响应。
// 输出：返回两个目标概念组及两个别名，但 Applied 为 false 且不执行替换 SQL。
// 副作用：读取模拟 SQLite 并调用固定模型，不写数据库。
func TestServiceRebuildSignalGroupsDryRunDoesNotWriteDatabase(t *testing.T) {
	// 1. 准备统计行、现有概念组和全覆盖模型响应。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COALESCE\\(analysis.recommendations_json").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recommendations_json", "risks_json", "market_mood", "prediction", "occurred_at"}).
			AddRow(`[{"name":"券商","type":"sector"},{"name":"黄金","type":"commodity"}]`, `[]`, "neutral", "range", "2026-07-20 10:00:00"))
	mock.ExpectQuery("SELECT g.id, g.canonical_name, g.group_type, a.alias_name").
		WillReturnRows(sqlmock.NewRows([]string{"id", "canonical_name", "group_type", "alias_name"}).
			AddRow(7, "证券行业", "sector", "券商"))
	gateway := &fixedSignalClassificationGateway{response: `{"groups":[` +
		`{"canonical_name":"证券行业","type":"sector","sources":[{"source_id":"group:7","confidence":0.98}]},` +
		`{"canonical_name":"贵金属","type":"commodity","sources":[{"source_id":"alias:0","confidence":0.95}]}],"pending":[]}`}

	// 2. 只读预演应返回可审查摘要，并在提示词中明确全局收敛目标。
	service := NewService(NewRepository(db), ServiceOptions{Model: "test-model", Analyzer: gateway})
	result, err := service.RebuildSignalGroups(context.Background(), 60, false)
	if err != nil {
		t.Fatalf("RebuildSignalGroups() error = %v", err)
	}
	if result.Applied || result.GroupCount != 2 || result.AliasCount != 2 || result.PendingAliasCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	for _, fragment := range []string{"20 到 40", "source_id", "每个来源必须且只能出现一次"} {
		if !strings.Contains(gateway.prompt, fragment) {
			t.Errorf("prompt is missing %q", fragment)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestRepositoryReplaceSignalGroupsAtomicallyRebuildsDictionary 验证全局词典只在单个事务中整体替换。
// 输入：一个新证券行业组和两个原始别名。
// 输出：先清空旧映射，再写入新组与别名并提交。
// 副作用：执行模拟 SQLite 删除和插入事务。
func TestRepositoryReplaceSignalGroupsAtomicallyRebuildsDictionary(t *testing.T) {
	// 1. 声明原子替换的全部 SQL 预期。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM investment_signal_alias").WillReturnResult(sqlmock.NewResult(0, 12))
	mock.ExpectExec("DELETE FROM investment_signal_group").WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec("INSERT INTO investment_signal_group").
		WithArgs("证券行业", "sector", "deepseek_rebuild", "test-model").
		WillReturnResult(sqlmock.NewResult(21, 1))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(21), "券商", "券商", 0.96, "deepseek_rebuild", "test-model").
		WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec("INSERT INTO investment_signal_alias").
		WithArgs(int64(21), "中信证券", "中信证券", 0.95, "deepseek_rebuild", "test-model").
		WillReturnResult(sqlmock.NewResult(32, 1))
	mock.ExpectCommit()

	// 2. 执行替换并核对写入数量与事务完整性。
	groups := []signalGroupProposal{{
		CanonicalName: "证券行业", Type: "sector",
		Aliases: []signalAliasProposal{{Name: "券商", Confidence: 0.96}, {Name: "中信证券", Confidence: 0.95}},
	}}
	count, err := NewRepository(db).ReplaceSignalGroups(context.Background(), groups, "test-model")
	if err != nil || count != 2 {
		t.Fatalf("ReplaceSignalGroups() = %d, %v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}
