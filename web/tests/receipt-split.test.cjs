const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");
function load() {
  const exports = {};
  vm.runInNewContext(
    ts.transpileModule(
      fs.readFileSync(require.resolve("../src/lib/receipt-split.ts"), "utf8"),
      { compilerOptions: { module: ts.ModuleKind.CommonJS } },
    ).outputText,
    {
      exports,
      require: () => ({ requestJSON: () => {} }),
      Date,
      URLSearchParams,
    },
  );
  return exports;
}
test("receipt ranges use Beijing calendar dates and exclusive end", () => {
  const lib = load();
  const range = lib.receiptRange(7, new Date("2026-10-21T16:05:00Z"));
  assert.equal(range.from, "2026-10-16");
  assert.equal(range.to, "2026-10-23");
  assert.equal(lib.shiftReceiptDate("2026-03-01", -1), "2026-02-28");
});
