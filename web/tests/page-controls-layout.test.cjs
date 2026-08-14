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
  assert.doesNotMatch(fetchSource, /RefreshCw|>\s*刷新\s*</);
});

test("微信读书绑定状态使用真实可用性文案", () => {
  const fetchSource = readSource("src/pages/finance/article-fetch.tsx");

  assert.match(fetchSource, /connected: "可用"/);
  assert.match(fetchSource, /failed: "已失效"/);
  assert.doesNotMatch(fetchSource, /connected: "已绑定"/);
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

test("综合趋势仅在指针进入图表后显示浮层并在左侧展示精简图例", () => {
  // 1. 读取综合趋势组件并检查指针跟随行为。
  const source = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");

  // 2. 确认鼠标移动会吸附日期，移出后隐藏准星和浮层。
  assert.match(source, /onPointerMove=\{handlePointerMove\}/);
  assert.match(source, /onPointerLeave=\{\(\) => setHoveredIndex\(null\)\}/);
  assert.match(source, /const activeIndex = hoveredIndex;/);
  assert.match(source, /activeIndex === null \? null : data\[activeIndex\]/);
  assert.match(source, /x1=\{activeChartPoint\.x\}/);
  assert.match(source, /y1=\{activeChartPoint\.y\}/);
  assert.match(source, /<rect x=\{tooltipX\} y=\{tooltipY\}/);
  assert.doesNotMatch(source, /onClick=\{handleChartClick\}/);

  // 3. 确认左侧图例包含文字说明，资产变化金额和百分比合并在浮层同一行。
  assert.match(source, /md:grid-cols-\[7rem_minmax\(0,1fr\)\]/);
  assert.match(source, /aria-hidden="true"/);
  assert.match(source, /\{item\.label\}\s*<\/span>/);
  assert.doesNotMatch(source, /<TrendMetricItem/);
  assert.match(source, /formatAssetChangePercent\(active\.total_asset, active\.daily_change\)/);
  assert.match(source, /fill=\{trendSeries\[2\]\.color\}[^>]*>\s*资产变化：\s*<tspan className=\{svgChangeTone\(active\.daily_change, isSensitiveMasked\)\}>\s*\{maskSensitive\(isSensitiveMasked, formatSignedCompactMoney\(active\.daily_change\)\)\}\s*（\{maskSensitive\(isSensitiveMasked, assetChangePercent\)\}）/);
  assert.doesNotMatch(source, />\s*资产变化率：/);
});

test("股票仓位分析的涨跌颜色使用红涨绿跌", () => {
  // 1. 读取股票仓位通用颜色函数和趋势浮层颜色函数。
  const formatSource = readSource("src/pages/finance/stock-analysis/format.ts");
  const trendSource = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");

  // 2. 正变化使用红色，负变化使用绿色。
  assert.match(formatSource, /if \(numberValue > 0\) \{\s*return "text-red-700";/);
  assert.match(formatSource, /if \(numberValue < 0\) \{\s*return "text-emerald-700";/);
  assert.match(trendSource, /if \(numberValue > 0\) \{\s*return "fill-red-700";/);
  assert.match(trendSource, /if \(numberValue < 0\) \{\s*return "fill-emerald-700";/);
});

test("综合趋势为资产变化使用独立Y轴", () => {
  // 1. 读取趋势图和类型定义，检查资产变化系列的轴归属。
  const source = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");
  const typesSource = readSource("src/pages/finance/stock-analysis/types.ts");

  // 2. 确认资产变化拥有独立范围、坐标换算和刻度标签。
  assert.match(typesSource, /axis: "money" \| "change" \| "percent"/);
  assert.match(source, /key: "daily_change"[^\n]+axis: "change"/);
  assert.match(source, /const changeDomain = paddedDomain\(changeValues, 1\)/);
  assert.match(source, /const changeY =/);
  assert.match(source, /series\.axis === "change"\s*\?\s*changeY/);
  assert.match(source, /formatSignedCompactMoney\(line\.changeValue\)/);
});

test("综合趋势不再为资产变化率绘制独立曲线和Y轴", () => {
  // 1. 读取趋势图和类型定义，检查派生变化率只用于浮层。
  const source = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");
  const typesSource = readSource("src/pages/finance/stock-analysis/types.ts");

  // 2. 确认变化率字段、曲线、坐标范围和刻度均已移除。
  assert.doesNotMatch(typesSource, /asset_change_percent/);
  assert.doesNotMatch(typesSource, /changePercent/);
  assert.doesNotMatch(typesSource, /dash\?: string/);
  assert.doesNotMatch(source, /key: "asset_change_percent"/);
  assert.doesNotMatch(source, /changePercentDomain|changePercentY|changePercentValue/);
  assert.doesNotMatch(source, /strokeDasharray=\{path\.dash\}/);
});

test("综合趋势Y轴刻度使用对应曲线颜色", () => {
  // 1. 读取趋势图组件，检查三组Y轴刻度颜色。
  const source = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");
  const moneyAxisStart = source.indexOf('{visibleAxes.has("money")');
  const changeAxisStart = source.indexOf('{visibleAxes.has("change")');
  const percentAxisStart = source.indexOf('{visibleAxes.has("percent")');
  const moneyAxisSource = source.slice(moneyAxisStart, changeAxisStart);
  const changeAxisSource = source.slice(changeAxisStart, percentAxisStart);
  const percentAxisSource = source.slice(percentAxisStart, source.indexOf("</g>", percentAxisStart));

  // 2. 金额、资产变化和仓位刻度分别使用黑色、绿色和橙色系列颜色。
  assert.ok(moneyAxisStart >= 0 && changeAxisStart > moneyAxisStart && percentAxisStart > changeAxisStart);
  assert.match(moneyAxisSource, /fill=\{trendSeries\[0\]\.color\}/);
  assert.match(changeAxisSource, /fill=\{trendSeries\[2\]\.color\}/);
  assert.match(percentAxisSource, /fill=\{trendSeries\[3\]\.color\}/);
});

test("综合趋势图例按钮可以控制曲线和Y轴显示", () => {
  // 1. 读取趋势图组件，检查图例开关状态和点击入口。
  const source = readSource("src/pages/finance/stock-analysis/trend-chart.tsx");

  // 2. 确认隐藏状态会同时过滤曲线，并控制对应轴刻度。
  assert.match(source, /useState<Set<TrendSeries\["key"\]>>\(new Set\(\)\)/);
  assert.match(source, /!hiddenSeries\.has\(item\.key\)/);
  assert.match(source, /<Toggle/);
  assert.match(source, /onPressedChange=\{\(\) => toggleSeries\(item\.key\)\}/);
  assert.match(source, /pressed=\{isVisible\}/);
  assert.match(source, /disabled=\{!isAvailable\}/);
  assert.match(source, /visibleAxes\.has\("change"\)/);
  assert.match(source, /visibleAxes\.has\("percent"\)/);
});

test("本地 Go 默认地址和 Vite API 代理统一使用 2345", () => {
  // 1. 读取 Vite 开发配置和公开环境变量模板。
  const viteSource = readSource("vite.config.ts");
  const envExample = readSource("../configs/.env.example");

  // 2. 确认浏览器代理与本地 Go 服务使用同一端口。
  assert.match(viteSource, /http:\/\/127\.0\.0\.1:2345/);
  assert.match(envExample, /^AOWUGONG_HTTP_ADDRESS=0\.0\.0\.0:2345$/m);
});
