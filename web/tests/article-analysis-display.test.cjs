const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const pageUtilsPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/page-utils.ts");
const pageSizePath = path.resolve(__dirname, "../src/pages/finance/article-analysis/use-responsive-table-page-size.ts");
const signalRankPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/signal-rank-card.tsx");
const articleAnalysisPagePath = path.resolve(__dirname, "../src/pages/finance/article-analysis/index.tsx");
const articlesCardPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/articles-card.tsx");
const articleDrawerPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/article-detail-drawer.tsx");

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

test("监控公众号列表只展示作者首字", () => {
  const { buildAccountStats } = loadPageUtils();
  const articles = [
    { title: "普通标题", author: "长江证券研究所", source_name: "公众号聚合" },
    { title: "另一标题", author: "长江证券研究所", source_name: "公众号聚合" },
  ];

  const stats = Array.from(buildAccountStats(articles));
  assert.equal(stats.length, 1);
  assert.equal(stats[0].name, "长");
  assert.equal(stats[0].count, 2);
});

test("桌面信号榜默认保留十五行位置", () => {
  const source = fs.readFileSync(pageSizePath, "utf8");
  assert.match(source, /DEFAULT_DENSE_TABLE_PAGE_SIZE\s*=\s*15\s*;/);
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
  assert.match(source, /<TableCell colSpan=\{4\}/);
  assert.match(source, /absolute right-0 top-0/);
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

test("概念组成员换行后分页只测真实行并允许降低行数", () => {
  const pageSizeSource = fs.readFileSync(pageSizePath, "utf8");
  const signalRankSource = fs.readFileSync(signalRankPath, "utf8");
  assert.match(pageSizeSource, /MIN_DENSE_TABLE_PAGE_SIZE\s*=\s*3\s*;/);
  assert.match(pageSizeSource, /tbody tr:not\(\[data-placeholder-row/);
  assert.match(signalRankSource, /data-placeholder-row="true"/);
});

test("桌面信号榜为完整成员和计数保留最小宽度", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(source, /xl:grid-cols-\[minmax\(20rem,0\.7fr\)_minmax\(0,1\.3fr\)\]/);
});

test("信号榜缩短标的列并为三位数字预留稳定列宽", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /w-\[42%\]/);
  assert.match(source, /w-\[25%\]/);
  assert.match(source, /w-\[15%\]/);
  assert.match(source, /w-\[18%\]/);
  assert.match(source, /grid-cols-\[42fr_25fr_15fr_18fr\]/);
  assert.match(source, /pr-9 text-right/);
});

test("手机端信号榜缩小数字并使用更紧凑的独立列比例", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /grid-cols-\[45fr_22fr_14fr_19fr\]\s+sm:grid-cols-\[42fr_25fr_15fr_18fr\]/);
  assert.match(source, /w-\[45%\][^"\n]*sm:w-\[42%\]/);
  assert.match(source, /gap-0\.5[^"\n]*sm:gap-1/);
  assert.match(source, /text-sm[^"\n]*sm:text-lg/);
  assert.match(source, /pr-8[^"\n]*sm:pr-9/);
  assert.match(source, /CardContent className="[^"\n]*px-3[^"\n]*sm:px-5/);
});

test("文章筛选加载完整六十天窗口而不是旧的两百篇", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(source, /fetchArticles\(TARGET_DAYS,\s*5000\)/);
});

test("超长概念成员在桌面信号榜内可以纵向滚动", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /xl:overflow-y-auto/);
  assert.match(source, /xl:\[scrollbar-gutter:stable\]/);
  assert.doesNotMatch(source, /xl:h-full xl:overflow-hidden/);
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
