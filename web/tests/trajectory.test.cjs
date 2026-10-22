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
      require: () => ({
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
test("GPSLogger configuration keeps upload secret out of URL and uses form seconds", () => {
  const { lib } = load();
  const c = lib.gpsLoggerConfig("https://example.com", "private-key");
  assert.equal(c.url, "https://example.com/api/v1/trajectory/collect");
  assert.ok(!c.url.includes("private-key"));
  assert.ok(c.headers.includes("Authorization: Bearer private-key"));
  assert.ok(c.body.includes("timestamp=%TIMESTAMP"));
  assert.ok(c.body.includes("spd=%SPD"));
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
