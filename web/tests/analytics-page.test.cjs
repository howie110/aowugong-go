const test = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const settle = () => new Promise((resolve) => setImmediate(resolve));

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

function elements(tree) {
  if (!React.isValidElement(tree)) return [];
  return [tree, ...React.Children.toArray(tree.props.children).flatMap(elements)];
}

const report = {
  current: { visitors: 2, visits: 3, views: 4, bounce_rate: 50, duration: 30 },
  previous: { visitors: 0, visits: 0, views: 0, bounce_rate: 0, duration: 0 },
  from: "2026-10-07T00:00:00+08:00",
  to: "2026-11-06T00:00:00+08:00",
  geo_enabled: false,
  trend: [
    { time: "2026-10-07T00:00:00+08:00", views: 1, visitors: 1 },
    { time: "2026-10-08T00:00:00+08:00", views: 3, visitors: 1 },
  ],
  ranks: {},
  heat: [],
};

test("访问统计只保留最近 30 天的按天趋势", async () => {
  const runtime = createRuntime();
  const calls = [];
  const api = {
    getAnalytics: async (from, to, filters, signal) => {
      calls.push([from, to, filters, Boolean(signal)]);
      return report;
    },
  };
  const { AnalyticsPage } = loadTypeScript("pages/analytics/index.tsx", {
    moduleMocks: {
      "pages/analytics/index.tsx": {
        react: runtime.hooks,
        "./charts": { Trend: () => "chart" },
      },
    },
    mocks: { "@/lib/analytics": api },
    globals: { AbortController },
  });
  const tree = await runtime.mount(() => AnalyticsPage());
  const html = renderToStaticMarkup(tree);

  assert.equal(calls.length, 1);
  const [from, to, filters, hasSignal] = calls[0];
  assert.equal(hasSignal, true);
  assert.equal(Object.keys(filters).length, 0);
  assert.equal(to.getTime() - from.getTime(), 30 * 86400000);
  assert.equal(from.getUTCHours(), 16);
  assert.equal(to.getUTCHours(), 16);
  assert.ok(html.includes("访问趋势"));
  assert.ok(html.includes("最近 30 天"));
  assert.ok(html.includes("按天"));
  assert.ok(html.includes("chart"));
  for (const removed of ["跳出率", "平均访问时长", "排除本浏览器", "时间范围", "筛选维度", "添加筛选", "地理分布", "流量时段", "与前一等长周期比较", "自定义"]) {
    assert.ok(!html.includes(removed), `unexpected ${removed}`);
  }

  const refresh = elements(tree).find((element) => element.props?.["aria-label"] === "刷新访问统计");
  assert.ok(refresh);
  refresh.props.onClick();
  await runtime.mount(() => AnalyticsPage());
  assert.equal(calls.length, 2);
});

test("访问趋势把每日数据映射到统一图表", () => {
  const { Trend } = loadTypeScript("pages/analytics/charts.tsx", {
    moduleMocks: {
      "pages/analytics/charts.tsx": { "./world.json": [] },
    },
    mocks: {
      "@/components/charts/trend-chart": {
        TrendChart: () => null,
      },
    },
  });
  const tree = Trend({ data: report.trend });
  assert.equal(tree.props.primaryLabel, "浏览量");
  assert.equal(tree.props.secondaryLabel, "访客");
  assert.deepEqual(JSON.parse(JSON.stringify(tree.props.data)), [
    { label: "2026-10-07", primary: 1, secondary: 1 },
    { label: "2026-10-08", primary: 3, secondary: 1 },
  ]);
});
