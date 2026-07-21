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

test("概念组按全部原始成员筛选文章", () => {
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
});

test("概念组灰字完整连接全部原始成员", () => {
  const { formatSignalMembers } = loadPageUtils();
  assert.equal(formatSignalMembers(["券商", "券商板块", "证券板块", "中信证券"]), "券商 · 券商板块 · 证券板块 · 中信证券");
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

test("文章筛选加载完整六十天窗口而不是旧的两百篇", () => {
  const source = fs.readFileSync(articleAnalysisPagePath, "utf8");
  assert.match(source, /fetchArticles\(TARGET_DAYS,\s*5000\)/);
});

test("超长概念成员在桌面信号榜内可以纵向滚动", () => {
  const source = fs.readFileSync(signalRankPath, "utf8");
  assert.match(source, /xl:overflow-y-auto/);
  assert.doesNotMatch(source, /xl:h-full xl:overflow-hidden/);
});
