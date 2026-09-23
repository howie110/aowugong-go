package articleanalysis

import (
	"encoding/json"
	"fmt"
)

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

输出前自检：recommendations 和 risks 中最终保留几个标的，signal_classifications 就必须返回几个不同的 name；不能遗漏任何一个，也不能提前截断 JSON。所有字符串必须正确转义，JSON 最外层必须完整闭合。

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
