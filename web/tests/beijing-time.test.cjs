const assert = require("node:assert/strict");
const test = require("node:test");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const beijing = loadTypeScript("lib/beijing-time.ts");

test("北京时间解析和格式化不依赖进程时区", () => {
  assert.equal(
    beijing.formatBeijingDateTime("2026-10-29 08:00:00", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }, "-", "sv-SE"),
    "08:00",
  );
  assert.equal(beijing.formatBeijingDate("2026-10-29 23:30:00"), "2026-10-29");
  assert.equal(beijing.formatBeijingInputDateTime(new Date("2026-10-21T16:01:00Z")), "2026-10-22T00:01");
});

test("北京时间今天和日期加减跨越月末和年末", () => {
  assert.equal(beijing.beijingToday(new Date("2026-10-21T16:01:00Z")), "2026-10-22");
  assert.equal(beijing.shiftBeijingDate("2026-12-31", 1), "2027-01-01");
  assert.equal(beijing.shiftBeijingDate("2026-03-01", -1), "2026-02-28");
});
