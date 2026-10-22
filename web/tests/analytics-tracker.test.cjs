const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");
function setup() {
  const exports = {};
  const calls = [];
  const events = {};
  const storage = new Map();
  const win = {
    location: {
      href: "https://aowugong.top/blog?token=secret",
      hostname: "aowugong.top",
    },
    document: {
      referrer: "https://google.com/search?q=secret",
      visibilityState: "visible",
    },
    navigator: { language: "zh-CN" },
    crypto: { randomUUID: () => "event-id" },
    localStorage: {
      getItem: (k) => storage.get(k),
      setItem: (k, v) => storage.set(k, v),
    },
    fetch: (...a) => {
      calls.push(a);
      return Promise.resolve({ ok: true });
    },
    addEventListener: (k, v) => (events[k] = v),
    history: {
      pushState(_s, _t, u) {
        win.location.href = new URL(u, win.location.href).href;
      },
      replaceState(_s, _t, u) {
        win.location.href = new URL(u, win.location.href).href;
      },
    },
  };
  const source = fs.readFileSync(
    require.resolve("../src/lib/analytics-tracker.ts"),
    "utf8",
  );
  vm.runInNewContext(
    ts.transpileModule(source, {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2020,
      },
    }).outputText,
    { exports, URL, URLSearchParams, queueMicrotask },
  );
  return { exports, win, calls, events, storage };
}
test("tracks initial and route navigation once, strips secrets, allows returning to page", async () => {
  const { exports, win, calls, events } = setup();
  exports.startAnalytics(win);
  exports.startAnalytics(win);
  assert.equal(calls.length, 1);
  assert.equal(JSON.parse(calls[0][1].body).url, "/blog");
  assert.equal(JSON.parse(calls[0][1].body).referrer, "https://google.com/");
  win.history.pushState({}, "", "/work");
  await Promise.resolve();
  assert.equal(calls.length, 2);
  win.history.replaceState({}, "", "/work#section");
  await Promise.resolve();
  assert.equal(calls.length, 2);
  win.location.href = "https://aowugong.top/blog";
  events.popstate();
  await Promise.resolve();
  assert.equal(calls.length, 3);
});
test("opt out and storage denial do not break navigation", async () => {
  const { exports, win, calls, storage } = setup();
  storage.set("aowugong.analytics.disabled", "1");
  exports.startAnalytics(win);
  assert.equal(calls.length, 0);
  storage.clear();
  win.localStorage.getItem = () => {
    throw Error("denied");
  };
  win.history.pushState({}, "", "/work");
  await Promise.resolve();
  assert.equal(calls.length, 1);
});

test("cancelled browser back does not count either transient URL", async () => {
  const { exports, win, calls, events } = setup();
  exports.startAnalytics(win);
  win.location.href = "https://aowugong.top/work";
  events.popstate();
  win.history.pushState({}, "", "/blog");
  await Promise.resolve();
  assert.equal(calls.length, 1);
});
