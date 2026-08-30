const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const pageUtilsPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/page-utils.ts");
const signalRankPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/signal-rank-card.tsx");
const signalNetTrendPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/signal-net-trend.tsx");
const articleAnalysisPagePath = path.resolve(__dirname, "../src/pages/finance/article-analysis/index.tsx");
const articlesCardPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/articles-card.tsx");
const articleDrawerPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/article-detail-drawer.tsx");
const articleFetchPagePath = path.resolve(__dirname, "../src/pages/finance/article-fetch.tsx");
const summaryCardsPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/summary-cards.tsx");
const pageConstantsPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/page-constants.ts");

/** 编译并加载不含运行时外部依赖的文章页面工具。 */
function loadPageUtils() {
  const source = fs.readFileSync(pageUtilsPath, "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText;
  const module = { exports: {} };
  vm.runInNewContext(output, { module, exports: module.exports });
  return module.exports;
}

test("分析页移除监控公众号并把提示词放入抓取页分析模型", () => {
  const analysisPage = fs.readFileSync(articleAnalysisPagePath, "utf8");
  const summaryCards = fs.readFileSync(summaryCardsPath, "utf8");
  const fetchPage = fs.readFileSync(articleFetchPagePath, "utf8");

  assert.doesNotMatch(`${analysisPage}\n${summaryCards}`, /监控公众号|模型和提示词|MonitoredAccountsCard|ModelPromptCard/);
  assert.match(fetchPage, /fetchArticleAnalysisModelSettings\(\)/);
  assert.match(fetchPage, /fetchArticleReport\(1, 1\)/);
  assert.match(fetchPage, /withPromptDetails\(nextModelSettings\)/);
  assert.match(fetchPage, /当前提示词/);
  assert.match(fetchPage, /<CollapsibleContent className="border-t">/);
  assert.match(fetchPage, /whitespace-pre-wrap break-words px-3 py-4 text-sm leading-6/);
  assert.doesNotMatch(fetchPage, /PopoverContent/);
  assert.match(fetchPage, /modelSettings\?\.prompt_version/);
  assert.match(fetchPage, /modelSettings\?\.analysis_prompt/);
});

test("抓取页三个操作展示后端真实结果而不是统一成功文案", () => {
  const fetchPage = fs.readFileSync(articleFetchPagePath, "utf8");

  assert.match(fetchPage, /buildArticleFetchNotification\(result\)/);
  assert.match(fetchPage, /buildArticleParseNotification\(result\)/);
  assert.match(fetchPage, /notification\.level === "warning"/);
});

test("短期市场判断始终补齐全部氛围和涨跌类别", () => {
  const { completeDistribution, formatDistributionValue, MARKET_MOOD_CATEGORIES, MARKET_PREDICTION_CATEGORIES } = loadPageUtils();
  const moods = completeDistribution([{ name: "neutral", count: 3 }], MARKET_MOOD_CATEGORIES);
  const predictions = completeDistribution([{ name: "up", count: 2 }], MARKET_PREDICTION_CATEGORIES);

  assert.equal(Array.from(moods, (item) => `${item.name}:${item.count}`).join(","), "very_optimistic:0,optimistic:0,neutral:3,pessimistic:0,very_pessimistic:0,unknown:0");
  assert.equal(Array.from(predictions, (item) => `${item.name}:${item.count}`).join(","), "up:2,range:0,down:0,unknown:0");
  assert.equal(formatDistributionValue(3, 6), "3/6 50%");
  assert.equal(formatDistributionValue(0, 6), "0/6 0%");
  assert.equal(formatDistributionValue(0, 0), "0/0 0%");

  const summary = fs.readFileSync(summaryCardsPath, "utf8");
  assert.match(summary, /completeDistribution\(report\?\.mood_distribution \|\| \[\], MARKET_MOOD_CATEGORIES\)/);
  assert.match(summary, /completeDistribution\(report\?\.prediction_distribution \|\| \[\], MARKET_PREDICTION_CATEGORIES\)/);
  assert.match(summary, /formatDistributionValue\(item\.count, total\)/);
  assert.match(summary, /transition-colors hover:bg-muted/);
});

test("信号榜和文章窗口统一统计最近九十天", () => {
  const constants = fs.readFileSync(pageConstantsPath, "utf8");
  const page = fs.readFileSync(articleAnalysisPagePath, "utf8");

  assert.match(constants, /TARGET_DAYS\s*=\s*90/);
  assert.match(page, /fetchArticleReport\(TARGET_DAYS, MARKET_DAYS\)/);
  assert.match(page, /fetchArticles\(TARGET_DAYS, 5000\)/);
  assert.match(page, /title=\{`信号榜 · \$\{TARGET_DAYS\}天`\}/);
});

test("概念组按全部成员筛选并支持精确筛选具体标的", () => {
  const { filterArticlesBySignal } = loadPageUtils();
  const articles = [
    { id: 1, recommendation_names: ["券商"], risk_names: [] },
    { id: 2, recommendation_names: [], risk_names: ["中信证券"] },
    { id: 3, recommendation_names: ["黄金"], risk_names: [] },
  ];
  const signal = {
    name: "证券行业",
    type: "sector",
    members: ["券商", "券商板块", "中信证券"],
    recommendation_count: 1,
    risk_count: 1,
    count: 2,
  };

  assert.deepEqual(Array.from(filterArticlesBySignal(articles, signal), (article) => article.id), [1, 2]);
  assert.deepEqual(Array.from(filterArticlesBySignal(articles, signal, "中信证券"), (article) => article.id), [2]);
});

test("概念组灰字使用斜杠紧凑连接全部原始成员", () => {
  const { formatSignalMembers } = loadPageUtils();
  assert.equal(formatSignalMembers(["券商", "券商板块", "证券板块", "中信证券"]), "券商 / 券商板块 / 证券板块 / 中信证券");
});

test("信号榜使用 Accordion 展开可点击的具体标的", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /components\/ui\/accordion/);
  assert.match(source, /components\/ui\/badge/);
  assert.match(source, /<Accordion/);
  assert.match(source, /<AccordionTrigger/);
  assert.match(source, /<AccordionContent/);
  assert.match(source, /<TableCell colSpan=\{5\}/);
  assert.match(source, /absolute right-1 top-14/);
  assert.doesNotMatch(source, /absolute right-1 top-1\/2/);
  assert.match(source, /item\.members\.map/);
  assert.match(source, /onSelectMember\(item, member\)/);
  assert.match(source, /selectedMember === member/);
  assert.match(source, /<Badge[^>]*asChild/);
});

test("信号榜标的群标签显示各自净数", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /item\.member_net_counts\?\.\[member\]\s*\?\?\s*0/);
  assert.match(source, /<SignalNetCell[^>]*compact/);
});

test("信号榜净数和总数列名旁提供升降序按钮", () => {
  const cardSource = fs.readFileSync(signalRankPath, "utf8");
  const pageSource = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(cardSource, /<SignalSortHeader label="净数" field="net"/);
  assert.match(cardSource, /<SignalSortHeader label="总数" field="total"/);
  assert.match(cardSource, /ArrowDown, ArrowUp, ArrowUpDown/);
  assert.match(pageSource, /field: "total",\s*direction: "desc"/);
  assert.match(pageSource, /setSignalPage\(1\)/);
});

test("信号榜排序作用于分页前的全部概念组", () => {
  const { sortSignals } = loadPageUtils();
  const signals = [
    { name: "甲", recommendation_count: 3, risk_count: 1, count: 4 },
    { name: "乙", recommendation_count: 1, risk_count: 5, count: 6 },
    { name: "丙", recommendation_count: 4, risk_count: 1, count: 5 },
  ];
  assert.equal(Array.from(sortSignals(signals, "total", "desc"), (item) => item.name).join(""), "乙丙甲");
  assert.equal(Array.from(sortSignals(signals, "net", "asc"), (item) => item.name).join(""), "乙甲丙");
});

test("信号榜占位行保持稳定行高", () => {
  const signalRankSource = fs.readFileSync(signalRankPath, "utf8");
  assert.match(signalRankSource, /data-placeholder-row="true"/);
});

test("信号榜和文章列表改为上下结构", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  const signalIndex = source.indexOf("<SignalRankCard");
  const articlesIndex = source.indexOf("<ArticlesCard");
  assert.ok(signalIndex >= 0 && articlesIndex > signalIndex);
  assert.match(source, /<div className="space-y-4 \[&>\*\]:min-w-0">/);
  assert.doesNotMatch(source, /xl:grid-cols-\[minmax\(20rem,0\.7fr\)_minmax\(0,1\.3fr\)\]/);
});

test("信号榜为每日净数图保留最大列宽和稳定行高", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /w-\[18%\]/);
  assert.match(source, /w-\[11%\]/);
  assert.match(source, /w-\[7%\]/);
  assert.match(source, /w-\[6%\]/);
  assert.match(source, /w-\[58%\]/);
  assert.match(source, /grid-cols-\[18fr_11fr_7fr_6fr_58fr\]/);
  assert.match(source, /min-h-28/);
  assert.match(source, /<SignalNetTrend points=\{item\.net_history \|\| \[\]\}/);
});

test("旧版报告可由文章列表补算九十天累计净数趋势", () => {
  const { withSignalNetHistory } = loadPageUtils();
  const signals = [
    {
      name: "证券行业",
      type: "sector",
      members: ["券商", "中信证券"],
      recommendation_count: 1,
      risk_count: 1,
      count: 2,
    },
  ];
  const articles = [
    { published_at: "2026-08-26", recommendation_names: ["券商"], risk_names: [] },
    { published_at: "2026-08-27", recommendation_names: [], risk_names: ["中信证券"] },
  ];

  const result = withSignalNetHistory(signals, articles, 3, new Date(2026, 7, 27));
  assert.equal(
    Array.from(result[0].net_history, (point) => `${point.date}:${point.net_count}`).join(","),
    "2026-08-25:0,2026-08-26:1,2026-08-27:0",
  );
  assert.equal(result[0].net_history.at(-1).net_count, result[0].recommendation_count - result[0].risk_count);
});

test("旧服务返回单日净变化时自动转换为累计净数", () => {
  const { withSignalNetHistory } = loadPageUtils();
  const signals = [{
    name: "人工智能",
    type: "theme",
    members: ["AI"],
    recommendation_count: 1,
    risk_count: 3,
    count: 4,
    net_history: [
      { date: "2026-08-25", net_count: -1 },
      { date: "2026-08-26", net_count: 0 },
      { date: "2026-08-27", net_count: -1 },
    ],
  }];

  const result = withSignalNetHistory(signals, [], 3, new Date(2026, 7, 27));
  assert.equal(
    Array.from(result[0].net_history, (point) => `${point.date}:${point.net_count}`).join(","),
    "2026-08-25:-1,2026-08-26:-1,2026-08-27:-2",
  );
  assert.equal(result[0].net_history.at(-1).net_count, result[0].recommendation_count - result[0].risk_count);
});

test("旧趋势缺少一条可落日信号时用起始基数对齐排行榜净数", () => {
  const { withSignalNetHistory } = loadPageUtils();
  const signals = [{
    name: "算力与半导体",
    type: "theme",
    members: ["算力"],
    recommendation_count: 75,
    risk_count: 76,
    count: 151,
    net_history: [
      { date: "2026-08-25", net_count: -1 },
      { date: "2026-08-26", net_count: -1 },
      { date: "2026-08-27", net_count: 0 },
    ],
  }];

  const result = withSignalNetHistory(signals, [], 3, new Date(2026, 7, 27));
  assert.equal(
    Array.from(result[0].net_history, (point) => `${point.date}:${point.net_count}`).join(","),
    "2026-08-25:0,2026-08-26:-1,2026-08-27:-1",
  );
});

test("小屏信号榜保持图表宽度并允许横向滚动", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /overflow-x-auto/);
  assert.match(source, /min-w-\[64rem\]/);
});

test("净数变化图悬停显示日期和净数坐标", () => {
  const source = fs.readFileSync(signalNetTrendPath, "utf8");
  assert.match(source, /block h-\[6\.5rem\] w-full/);
  assert.match(source, /new ResizeObserver\(updateWidth\)/);
  assert.match(source, /right: chartWidth - 10/);
  assert.match(source, /onPointerMove=\{handlePointerMove\}/);
  assert.match(source, /onPointerLeave=\{\(\) => setHoveredIndex\(null\)\}/);
  assert.match(source, /\{active\.date\}/);
  assert.match(source, /净数 \{formatSigned\(active\.net_count\)\}/);
  assert.match(source, /strokeDasharray="3 3"/);
});

test("文章筛选加载完整九十天窗口而不是旧的两百篇", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(source, /fetchArticles\(TARGET_DAYS,\s*5000\)/);
});

test("信号榜固定每页八行以容纳清晰趋势图", () => {
  const pageSource = fs.readFileSync(articleAnalysisPagePath, "utf8");
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(pageSource, /SIGNAL_PAGE_SIZE\s*=\s*8/);
  assert.match(source, /h-28/);
  assert.doesNotMatch(source, /xl:overflow-y-auto/);
});

test("文章列表固定每页二十行", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(source, /ARTICLE_PAGE_SIZE\s*=\s*20/);
  assert.match(source, /filteredArticles\.slice\(start, start \+ ARTICLE_PAGE_SIZE\)/);
  assert.match(source, /pageSize=\{ARTICLE_PAGE_SIZE\}/);
});

test("文章列表移除标的群并用 Breadcrumb 展示筛选位置", () => {
  const source = fs.readFileSync(articlesCardPath, "utf8");
  assert.match(source, /components\/ui\/breadcrumb/);
  assert.match(source, /<Breadcrumb/);
  assert.match(source, /<BreadcrumbPage>\{selectedMember\}<\/BreadcrumbPage>/);
  assert.match(source, /onSelectSignalGroup/);
  assert.doesNotMatch(source, /data-article-signal-groups/);
  assert.doesNotMatch(source, /ArticleSignalGroupFilters/);
  assert.doesNotMatch(source, /signalItems/);
});

test("文章 Breadcrumb 返回父级不修改信号榜的页码和选中态", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  const handlerStart = source.indexOf("function handleArticleBreadcrumbChange");
  const handlerEnd = source.indexOf("function handleArticleDetailChange");
  const handlerSource = source.slice(handlerStart, handlerEnd);

  assert.ok(handlerStart >= 0);
  assert.match(source, /selectedRankSignal/);
  assert.match(source, /selectedRankMember/);
  assert.match(source, /selectedArticleSignal/);
  assert.match(source, /selectedArticleMember/);
  assert.match(source, /onSelectSignalGroup=\{\(\) => handleArticleBreadcrumbChange\("group"\)\}/);
  assert.doesNotMatch(handlerSource, /setSignalPage|setSelectedRankSignal|setSelectedRankMember/);
});

test("信号榜具体标的按钮同步筛选文章", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  const handlerStart = source.indexOf("function handleSelectRankMember");
  const handlerEnd = source.indexOf("function handleArticleBreadcrumbChange");
  const handlerSource = source.slice(handlerStart, handlerEnd);

  assert.ok(handlerStart >= 0);
  assert.match(handlerSource, /setSelectedRankSignal\(signal\)/);
  assert.match(handlerSource, /setSelectedRankMember\(nextMember\)/);
  assert.match(handlerSource, /setSelectedArticleSignal\(signal\)/);
  assert.match(handlerSource, /setSelectedArticleMember\(nextMember\)/);
  assert.match(source, /onSelectMember=\{handleSelectRankMember\}/);
});

test("文章明细使用 Sheet 和 Textarea", () => {
  const source = fs.readFileSync(articleDrawerPath, "utf8");
  assert.match(source, /<SheetContent[^>]*side="left"/);
  assert.match(source, /<Textarea/);
  assert.doesNotMatch(source, /fixed inset-0 z-50/);
});
