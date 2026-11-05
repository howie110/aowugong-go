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

function elements(tree) {
  if (!React.isValidElement(tree)) return [];
  return [tree, ...React.Children.toArray(tree.props.children).flatMap(elements)];
}

function text(tree) {
  if (typeof tree === "string" || typeof tree === "number") return String(tree);
  if (!React.isValidElement(tree)) return "";
  return React.Children.toArray(tree.props.children).map(text).join("");
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
    moduleMocks: { "pages/receipt-split/index.tsx": { react: runtime.hooks } },
    mocks: { "@/lib/receipt-split": api },
    globals: { AbortController },
  });
  const tree = await runtime.mount(() => ReceiptSplitPage());
  const html = renderToStaticMarkup(tree);

  assert.deepEqual(rangeCalls, [30]);
  assert.deepEqual(statisticsCalls, [["2026-10-07", "2026-11-06", true, true]]);
  assert.ok(html.includes("每日使用用户"));
  assert.ok(html.includes("最近 30 天"));
  for (const removed of ["历史登录用户", "识别任务", "识别成功率", "开始日期", "结束日期", "排除运营者", "每日明细", "数据生成"]) {
    assert.ok(!html.includes(removed), `unexpected ${removed}`);
  }
  assert.ok(html.includes("<svg"));
});

test("趋势图鼠标移动显示十字准线和最近日期提示", () => {
  const runtime = createRuntime();
  const { ReceiptTrend } = loadTypeScript("pages/receipt-split/trend.tsx", {
    moduleMocks: { "pages/receipt-split/trend.tsx": { react: runtime.hooks } },
  });
  const render = () => {
    runtime.resetCursor();
    return ReceiptTrend({ days });
  };

  let tree = render();
  const svg = elements(tree).find((element) => element.type === "svg");
  assert.ok(svg);
  svg.props.onPointerMove({
    currentTarget: { getBoundingClientRect: () => ({ left: 0, top: 0, width: 980, height: 260 }) },
    clientX: 500,
    clientY: 100,
  });
  tree = render();

  const dashed = elements(tree).filter((element) => element.type === "line" && element.props.strokeDasharray === "4 4");
  assert.equal(dashed.length, 2);
  assert.equal(dashed[0].props.x1, 500);
  assert.equal(dashed[1].props.y1, 100);
  const labels = elements(tree).filter((element) => element.type === "text").map(text);
  assert.ok(labels.includes("2026-10-02"));
  assert.ok(labels.includes("识别用户 3"));
  assert.ok(labels.includes("生成账单用户 1"));
  const tooltip = elements(tree).find((element) => element.type === "rect" && element.props.width === 168);
  assert.equal(tooltip.props.fill, "#ffffff");
  const dateLabel = elements(tree).find((element) => element.type === "text" && text(element) === "2026-10-02");
  assert.equal(dateLabel.props.fill, "#0f172a");

  svg.props.onPointerLeave();
  tree = render();
  assert.equal(elements(tree).filter((element) => element.type === "line" && element.props.strokeDasharray === "4 4").length, 0);
});
