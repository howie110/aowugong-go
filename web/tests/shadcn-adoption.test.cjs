const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const srcRoot = path.resolve(__dirname, "../src");

/** readSource 读取前端源码并返回文本，无副作用。 */
function readSource(relativePath) {
  // 1. 从前端源码根目录解析并读取目标文件。
  return fs.readFileSync(path.join(srcRoot, relativePath), "utf8");
}

test("补齐当前页面需要的 shadcn 基础组件", () => {
  const components = [
    "accordion",
    "alert",
    "alert-dialog",
    "attachment",
    "avatar",
    "button-group",
    "calendar",
    "checkbox",
    "collapsible",
    "dialog",
    "dropdown-menu",
    "empty",
    "field",
    "input-group",
    "item",
    "pagination",
    "popover",
    "progress",
    "scroll-area",
    "select",
    "spinner",
    "textarea",
    "toggle",
    "toggle-group",
  ];

  for (const component of components) {
    assert.equal(fs.existsSync(path.join(srcRoot, "components/ui", `${component}.tsx`)), true, `${component}.tsx 应存在`);
  }
});

test("全局布局使用完整 Sidebar 和 Tooltip 组合", () => {
  const shell = readSource("components/layout/app-shell.tsx");
  const navigation = readSource("components/layout/app-sidebar-navigation.tsx");
  const main = readSource("main.tsx");

  assert.match(shell, /SidebarRail/);
  assert.match(shell, /collapsible="icon"/);
  assert.match(navigation, /CollapsibleTrigger/);
  assert.match(navigation, /SidebarMenuItem/);
  assert.match(main, /TooltipProvider/);
});

test("面包屑恢复默认分隔并使用 shadcn 图标按钮", () => {
  const breadcrumb = readSource("components/layout/app-breadcrumb.tsx");

  assert.match(breadcrumb, /from "@\/components\/ui\/button"/);
  assert.match(breadcrumb, /TooltipContent/);
  assert.doesNotMatch(breadcrumb, /\{" > "\}/);
});

test("登录和权限页面使用 shadcn 表单控件", () => {
  const login = readSource("pages/login.tsx");
  const permissions = readSource("pages/permissions.tsx");
  assert.match(login, /components\/ui\/checkbox/);
  assert.doesNotMatch(login, /type="checkbox"/);
  assert.match(permissions, /SelectTrigger/);
  assert.doesNotMatch(permissions, /<select/);
});

test("日期和文件输入使用统一的 shadcn 组合", () => {
  const datePicker = readSource("components/date-picker.tsx");
  const subscriptions = readSource("pages/subscriptions.tsx");
  const mahjong = readSource("pages/mahjong.tsx");
  const positionUpload = readSource("pages/finance/position-upload.tsx");
  assert.match(datePicker, /PopoverTrigger/);
  assert.match(datePicker, /Calendar/);
  assert.doesNotMatch(`${subscriptions}\n${mahjong}\n${positionUpload}`, /type="date"/);
  assert.match(positionUpload, /type="file"[\s\S]*className="sr-only"/);
  assert.match(positionUpload, /Attachment/);
});

test("订阅删除使用 AlertDialog 确认", () => {
  const subscriptions = readSource("pages/subscriptions.tsx");
  assert.match(subscriptions, /AlertDialogContent/);
  assert.doesNotMatch(subscriptions, /window\.confirm/);
});

test("文章汇总、模型提示词和分页使用 shadcn 交互组件", () => {
  const summary = readSource("pages/finance/article-analysis/summary-cards.tsx");
  const fetch = readSource("pages/finance/article-fetch.tsx");
  const pagination = readSource("pages/finance/article-analysis/article-pagination.tsx");
  assert.match(summary, /<Progress/);
  assert.match(fetch, /CollapsibleContent/);
  assert.doesNotMatch(fetch, /PopoverContent/);
  assert.match(pagination, /components\/ui\/pagination/);
  assert.doesNotMatch(pagination, /lucide-react/);
});

test("工作导航使用 InputGroup、ToggleGroup、Empty 和 Tooltip", () => {
  const work = readSource("pages/work.tsx");
  assert.match(work, /<InputGroup/);
  assert.match(work, /<ToggleGroup/);
  assert.match(work, /<Empty/);
  assert.match(work, /<Tooltip/);
  assert.doesNotMatch(work, /<button/);
});

test("股票分析使用 Toggle、Progress 和 Empty", () => {
  const trend = readSource("pages/finance/stock-analysis/trend-chart.tsx");
  const holdings = readSource("pages/finance/stock-analysis/holding-distribution-card.tsx");
  const empty = readSource("pages/finance/stock-analysis/empty-analysis.tsx");
  assert.match(trend, /<Toggle/);
  assert.doesNotMatch(trend, /<button/);
  assert.match(holdings, /<Progress/);
  assert.match(empty, /<Empty/);
});

test("抓取、监控和总览页面使用统一状态组件", () => {
  const articleFetch = readSource("pages/finance/article-fetch.tsx");
  const monitoring = readSource("pages/monitoring.tsx");
  const overview = readSource("pages/dashboard/finance-content.tsx");
  assert.match(articleFetch, /<ButtonGroup/);
  assert.match(articleFetch, /<Skeleton/);
  assert.match(monitoring, /<Skeleton/);
  assert.match(monitoring, /<Empty/);
  assert.match(overview, /<Alert/);
  assert.match(overview, /<Skeleton/);
});

test("麻将和微信读书使用 Progress、Empty、Skeleton 与 Spinner", () => {
  const mahjong = readSource("pages/mahjong.tsx");
  const weread = readSource("pages/weread.tsx");
  assert.match(mahjong, /<Progress/);
  assert.match(mahjong, /<Empty/);
  assert.match(mahjong, /<Spinner/);
  assert.match(weread, /<Progress/);
  assert.match(weread, /<Empty/);
  assert.match(weread, /<Skeleton/);
});

test("敏感数据开关使用 Toggle 和 Tooltip", () => {
  const header = readSource("pages/dashboard/page-header.tsx");
  assert.match(header, /<Toggle/);
  assert.match(header, /<Tooltip/);
});

test("只读数据库页面使用 shadcn 数据浏览组件", () => {
  const database = readSource("pages/database.tsx");
  const finance = readSource("lib/finance.ts");
  const navigation = readSource("components/layout/app-navigation.ts");
  assert.match(database, /<Table>/);
  assert.match(database, /<Select/);
  assert.match(database, /<InputGroup/);
  assert.match(database, /<Pagination/);
  assert.match(database, /<ScrollArea/);
  assert.doesNotMatch(database, /<table/);
  assert.match(finance, /database: "page:database"/);
  assert.match(navigation, /key: "database"/);
});
