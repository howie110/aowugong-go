const test = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const settle = () => new Promise((resolve) => setImmediate(resolve));

// 只替换页面 hooks，子组件仍走真实实现。
function createRuntime() {
  const values = [];
  const effects = [];
  let cursor = 0;
  let dirty = false;

  const hooks = {
    useState(initial) {
      const slot = cursor++;
      if (!(slot in values)) values[slot] = typeof initial === "function" ? initial() : initial;
      return [values[slot], (next) => {
        const value = typeof next === "function" ? next(values[slot]) : next;
        if (!Object.is(value, values[slot])) {
          values[slot] = value;
          dirty = true;
        }
      }];
    },
    useEffect(effect, deps) {
      const slot = cursor++;
      const previous = effects[slot];
      const changed = !previous || !deps || !previous.deps || deps.length !== previous.deps.length
        || deps.some((value, index) => !Object.is(value, previous.deps[index]));
      effects[slot] = { effect, deps };
      if (changed) effect();
    },
  };

  return {
    hooks,
    resetCursor() { cursor = 0; },
    async mount(render) {
      let tree;
      for (let attempt = 0; attempt < 30; attempt += 1) {
        dirty = false;
        cursor = 0;
        tree = render();
        await settle();
        if (!dirty) break;
      }
      return tree;
    },
  };
}

const days = [
  { date: "2026-10-01", analysis_users: 1, bill_users: 0, tasks: 1, succeeded: 1, failed: 0, running: 0, queued: 0, bills: 0 },
  { date: "2026-10-02", analysis_users: 3, bill_users: 1, tasks: 3, succeeded: 3, failed: 0, running: 0, queued: 0, bills: 1 },
  { date: "2026-10-03", analysis_users: 2, bill_users: 2, tasks: 2, succeeded: 2, failed: 0, running: 0, queued: 0, bills: 2 },
];

test("小票页只保留最近 30 天的每日使用图", async () => {
  const runtime = createRuntime();
  const rangeCalls = [];
  const statisticsCalls = [];
  const api = {
    receiptRange: (days) => {
      rangeCalls.push(days);
      return { from: "2026-10-07", to: "2026-11-06" };
    },
    getReceiptStatistics: async (from, to, exclude, signal) => {
      statisticsCalls.push([from, to, exclude, Boolean(signal)]);
      return {
        version: 1,
        from,
        to,
        exclude_operators: exclude,
        operators_configured: false,
        generated_at: "2026-11-05T10:00:00+08:00",
        lifetime_login_users: 16,
        totals: { analysis_users: 3, tasks: 4, succeeded: 4, failed: 0, running: 0, queued: 0, bill_users: 3, bills: 3 },
        days,
      };
    },
  };
  const { ReceiptSplitPage } = loadTypeScript("pages/receipt-split/index.tsx", {
    moduleMocks: {
      "pages/receipt-split/index.tsx": {
        react: runtime.hooks,
        "./trend": { ReceiptTrend: () => "chart" },
      },
    },
    mocks: { "@/lib/receipt-split": api },
    globals: { AbortController },
  });
  const tree = await runtime.mount(() => ReceiptSplitPage());
  const html = renderToStaticMarkup(tree);

  assert.deepEqual(rangeCalls, [30]);
  assert.deepEqual(statisticsCalls, [["2026-10-07", "2026-11-06", true, true]]);
  assert.ok(html.includes("每日使用用户"));
  assert.ok(html.includes("最近 30 天"));
  assert.ok(html.includes("chart"));
  for (const removed of ["历史登录用户", "识别任务", "识别成功率", "开始日期", "结束日期", "排除运营者", "每日明细", "数据生成"]) {
    assert.ok(!html.includes(removed), `unexpected ${removed}`);
  }
});

test("小票趋势把每日数据映射到统一图表", () => {
  const { ReceiptTrend } = loadTypeScript("pages/receipt-split/trend.tsx", {
    mocks: {
      "@/components/charts/trend-chart": {
        TrendChart: () => null,
      },
    },
  });
  const tree = ReceiptTrend({ days });
  assert.equal(tree.props.primaryLabel, "识别用户");
  assert.equal(tree.props.secondaryLabel, "生成账单用户");
  assert.deepEqual(JSON.parse(JSON.stringify(tree.props.data)), [
    { label: "2026-10-01", primary: 1, secondary: 0 },
    { label: "2026-10-02", primary: 3, secondary: 1 },
    { label: "2026-10-03", primary: 2, secondary: 2 },
  ]);
});
