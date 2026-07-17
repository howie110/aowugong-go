const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

// readSource 读取指定前端源码，供页面结构回归测试检查。
function readSource(relativePath) {
  // 1. 从 web 目录解析并读取 UTF-8 源文件。
  return fs.readFileSync(path.resolve(__dirname, "..", relativePath), "utf8");
}

test("共享页头不再显示右上角日期", () => {
  const pageHeaderSource = readSource("src/pages/dashboard/page-header.tsx");
  const dashboardSource = readSource("src/pages/dashboard.tsx");

  assert.doesNotMatch(pageHeaderSource, /toLocaleDateString|<Badge/);
  assert.doesNotMatch(dashboardSource, /isLoading=\{isPageLoading\}/);
});

test("浏览器标签使用工作台名称和应用图标", () => {
  const indexSource = readSource("index.html");
  const faviconPath = path.resolve(__dirname, "../public/favicon.svg");

  assert.match(indexSource, /<title>Aowugong 工作台<\/title>/);
  assert.match(indexSource, /<link rel="icon" type="image\/svg\+xml" href="\/favicon\.svg" \/>/);
  assert.equal(fs.existsSync(faviconPath), true);
});

test("文章分析和文章抓取页面不再显示刷新按钮", () => {
  const analysisSource = readSource("src/pages/finance/article-analysis/index.tsx");
  const fetchSource = readSource("src/pages/finance/article-fetch.tsx");

  assert.doesNotMatch(analysisSource, /RefreshCw|刷新/);
  assert.doesNotMatch(fetchSource, /RefreshCw|刷新/);
});

test("抓取和分析操作按钮使用相同固定宽度", () => {
  const fetchSource = readSource("src/pages/finance/article-fetch.tsx");
  const fixedWidthButtons = fetchSource.match(/className="w-28"/g) || [];

  assert.equal(fixedWidthButtons.length, 2);
});

test("导入记录在桌面和手机端都隐藏账户与券商", () => {
  const source = readSource("src/pages/finance/position-upload.tsx");
  const tableStart = source.indexOf("function SnapshotTable");
  const tableEnd = source.indexOf("function ImportMeta");
  const snapshotTableSource = source.slice(tableStart, tableEnd);

  assert.ok(tableStart >= 0 && tableEnd > tableStart);
  assert.doesNotMatch(snapshotTableSource, /账户|券商|account_alias|account_suffix|broker_name/);
});
