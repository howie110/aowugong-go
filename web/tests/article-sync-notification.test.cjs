const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const modulePath = path.resolve(__dirname, "../src/lib/article-sync-notification.ts");

/** 编译并加载文章同步通知 TypeScript 模块。 */
function loadModule() {
  assert.equal(fs.existsSync(modulePath), true, "缺少文章同步通知模块");

  const source = fs.readFileSync(modulePath, "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const module = { exports: {} };

  vm.runInNewContext(output, { module, exports: module.exports });
  return module.exports;
}

/** 返回满足通知函数输入要求的同步结果。 */
function createSyncResult(failedSources = []) {
  return {
    inserted_count: 2,
    updated_count: 3,
    analyzed_count: 4,
    failed_sources: failedSources,
  };
}

test("全部来源成功时生成成功通知", () => {
  const { buildArticleSyncNotification } = loadModule();

  const result = buildArticleSyncNotification(createSyncResult());

  assert.equal(result.level, "success");
  assert.equal(result.title, "抓取并分析完成");
  assert.match(result.description, /失败来源 0 个/);
});

test("存在失败来源时生成包含原因的警告通知", () => {
  const { buildArticleSyncNotification } = loadModule();

  const result = buildArticleSyncNotification(
    createSyncResult([{ source: "公众号聚合", error: "轮询器未启动" }]),
  );

  assert.equal(result.level, "warning");
  assert.match(result.title, /存在失败来源/);
  assert.match(result.description, /公众号聚合：轮询器未启动/);
});
