const assert = require("node:assert/strict");
const test = require("node:test");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

function client(entry, respond) {
  const calls = [];
  const api = loadTypeScript(entry, { mocks: { "@/lib/auth": { authorizedFetch: async (url, init = {}) => {
    calls.push({ url, init });
    return respond(url, init, calls.length);
  } } } });
  return { api, calls };
}

test("仓位接口保留查询参数和 multipart 字段，不强设 JSON 请求头", async () => {
  const { api, calls } = client("lib/positions.ts", () => Response.json({ results: [] }));
  await api.fetchRecentSnapshots();
  const result = await api.uploadPositionSnapshots({ snapshotDate: "2026-09-23", brokerName: "东莞证券", sourceApp: "同花顺", files: [new Blob(["image"], { type: "image/png" })] });
  assert.equal(calls[0].url, "/api/v1/finance/positions/snapshots/recent?limit=20");
  assert.equal(calls[1].url, "/api/v1/finance/positions/snapshots/upload");
  assert.equal(calls[1].init.method, "POST");
  assert.equal(new Headers(calls[1].init.headers).has("Content-Type"), false);
  const body = calls[1].init.body;
  assert.equal(body.get("snapshot_date"), "2026-09-23");
  assert.equal(body.get("broker_name"), "东莞证券");
  assert.equal(body.get("source_app"), "同花顺");
  assert.equal(await body.getAll("files")[0].text(), "image");
  assert.equal(result.results.length, 0);
});

test("股票报告保留默认 limit 和原有失败提示", async () => {
  const { api, calls } = client("lib/stock-analysis.ts", () => Response.json({ detail: "内部细节" }, { status: 500 }));
  await assert.rejects(api.fetchStockAnalysisReport(), /读取股票仓位分析失败/);
  assert.equal(calls[0].url, "/api/v1/finance/stock-analysis/report?limit=500");
});

test("公共 JSON 请求保留错误状态与回退提示，不重试", async () => {
  const { api, calls } = client("lib/request.ts", () => new Response("invalid", { status: 503 }));
  await assert.rejects(api.requestJSON("/test", {}, "操作失败"), (error) => error.message === "操作失败" && error.status === 503);
  assert.equal(calls.length, 1);
});

test("文章反馈仅在 405 时沿原有路径兼容重试，保留 JSON 请求头", async () => {
  const { api, calls } = client("lib/article-analysis.ts", (_url, _init, count) => count === 1
    ? Response.json({ detail: "method" }, { status: 405 }) : Response.json({ id: 7 }));
  assert.equal((await api.saveArticlePromptFeedback(7, "反馈")).id, 7);
  assert.equal(calls.length, 2);
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[1].init.method, "PATCH");
  assert.equal(calls[0].url, calls[1].url);
  assert.equal(new Headers(calls[1].init.headers).get("Content-Type"), "application/json");
  const failure = client("lib/article-analysis.ts", () => Response.json({ detail: "拒绝" }, { status: 403 }));
  await assert.rejects(failure.api.saveArticlePromptFeedback(7, "反馈"), /拒绝/);
  assert.equal(failure.calls.length, 1);
});

test("VPN 二维码保持 Blob，JSON 操作保留服务端错误", async () => {
  const { api, calls } = client("lib/vpn.ts", (url) => url.includes("/qr?")
    ? new Response("png", { headers: { "Content-Type": "image/png" } })
    : Response.json({ detail: "资源不可用" }, { status: 400 }));
  const blob = await api.fetchVPNQRCode(3, "surge");
  assert.equal(blob.type, "image/png");
  assert.equal(await blob.text(), "png");
  assert.equal(calls[0].url, "/api/v1/vpn/resources/users/3/qr?format=surge");
  await assert.rejects(api.createVPNUserSubscription(2, "dmit"), /资源不可用/);
  assert.deepEqual(JSON.parse(calls[1].init.body), { user_id: 2, profile_code: "dmit" });
});
