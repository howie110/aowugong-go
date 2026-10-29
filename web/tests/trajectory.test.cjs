const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");
function load() {
  const exports = {};
  const calls = [];
  vm.runInNewContext(
    ts.transpileModule(
      fs.readFileSync(require.resolve("../src/lib/trajectory.ts"), "utf8"),
      { compilerOptions: { module: ts.ModuleKind.CommonJS } },
    ).outputText,
    {
      exports,
      require: (name) => name === "@/lib/beijing-time"
        ? require("./helpers/beijing-time.cjs")
        : ({
          requestJSON: (...x) => {
            calls.push(x);
            return Promise.resolve({});
          },
        }),
      Date,
      URLSearchParams,
    },
  );
  return { lib: exports, calls };
}
test("trajectory dates follow Beijing boundaries", () => {
  const { lib } = load();
  assert.equal(
    lib.trajectoryToday(new Date("2026-10-21T16:01:00Z")),
    "2026-10-22",
  );
  assert.equal(lib.nextTrajectoryDay("2026-12-31"), "2027-01-01");
});
test("existing device configuration uses profile endpoint without rotating its key", async () => {
  const { lib, calls } = load();
  await lib.getTrajectoryProfile("device/1");
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/api/v1/trajectory/devices/device%2F1/profile");
  assert.equal(calls[0][1].method, "POST");
});
test("trajectory client encodes dates and snapshot; device credentials use body", () => {
  const { lib, calls } = load();
  lib.getTrajectory({
    from: "2026-10-21",
    through: "2026-10-22",
    device: "",
    page: 2,
    snapshot: "2026-10-22T01:00:00Z",
  });
  assert.ok(calls[0][0].includes("to=2026-10-23"));
  assert.ok(calls[0][0].includes("page=2"));
  lib.createTrajectoryDevice("手机");
  assert.equal(calls[1][1].method, "POST");
  assert.equal(JSON.parse(calls[1][1].body).name, "手机");
});

const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { loadTypeScript } = require('./helpers/load-typescript.cjs');
const settle = () => new Promise(resolve => setImmediate(resolve));
async function trajectoryPage({ clipboardFails = false, profileFails = false } = {}) {
  const states = [], effects = [], calls = [];
  let cursor = 0, mounted = false;
  const device = { id: 'existing-device', name: '测试手机', revoked_at: null, last_received_at: null };
  const hooks = { ...React, useState(initial) {
    const index = cursor++;
    if (!(index in states)) states[index] = typeof initial === 'function' ? initial() : initial;
    return [states[index], value => { states[index] = typeof value === 'function' ? value(states[index]) : value; }];
  }, useEffect(effect) { if (!mounted) effects.push(effect); } };
  const { TrajectoryPage } = loadTypeScript('pages/trajectory/index.tsx', {
    moduleMocks: { 'pages/trajectory/index.tsx': { react: hooks } },
    globals: { AbortController, window: { location: { origin: 'https://example.test' } }, navigator: { clipboard: { writeText: async value => { if (clipboardFails) throw new Error('clipboard blocked'); calls.push(['copy',value]); } } } },
    mocks: { '@/lib/trajectory': {
      trajectoryToday: () => '2026-10-22',
      getTrajectoryDevices: async () => [device],
      getTrajectory: async () => ({ points: [], total: 0, page_size: 50, today_count: 0, last_recorded_at: null, last_received_at: null }),
      getTrajectoryProfile: async id => { calls.push(['profile', id]); if (profileFails) throw new Error('生成配置失败'); return { profile_path: '/profile/ticket/aowugong.properties' }; },
      createTrajectoryDevice: async name => { calls.push(['create', name]); return { device }; },
    } },
  });
  const render = () => { cursor = 0; return TrajectoryPage(); };
  render(); mounted = true; effects.forEach(effect => effect()); await settle();
  return { render, calls };
}
function findControl(tree, label) {
  if (!React.isValidElement(tree)) return null;
  if (tree.props.onClick && React.Children.toArray(tree.props.children).some(child => typeof child === 'string' && child.includes(label))) return tree;
  for (const child of React.Children.toArray(tree.props.children)) { const found = findControl(child,label); if (found) return found; }
  return null;
}
for (const options of [{}, {clipboardFails:true}, {profileFails:true}]) {
  test(`trajectory configuration copy handles ${JSON.stringify(options)}`, async () => {
    const p = await trajectoryPage(options);
    const copy = findControl(p.render(),'复制配置'); assert.ok(copy);
    await copy.props.onClick(); await settle();
    assert.deepEqual(p.calls[0], ['profile','existing-device']);
    const html = renderToStaticMarkup(p.render());
    if (options.profileFails) assert.ok(html.includes('生成配置失败'));
    else if (options.clipboardFails) assert.ok(html.includes('https://example.test/profile/ticket/aowugong.properties'));
    else assert.deepEqual(p.calls[1], ['copy','https://example.test/profile/ticket/aowugong.properties']);
    assert.ok(!html.includes('更换密钥'));
  });
}
