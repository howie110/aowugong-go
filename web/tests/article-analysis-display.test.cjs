const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const pageUtilsPath = path.resolve(__dirname, "../src/pages/finance/article-analysis/page-utils.ts");
const pageSizePath = path.resolve(__dirname, "../src/pages/finance/article-analysis/use-responsive-table-page-size.ts");

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
