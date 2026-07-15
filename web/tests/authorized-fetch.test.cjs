const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const modulePath = path.resolve(__dirname, "../src/lib/auth.ts");

/** 编译认证模块并注入可观察的浏览器和 fetch。 */
function loadAuthModule({ token = "test-token", status = 200 } = {}) {
  const source = fs.readFileSync(modulePath, "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText;
  const values = new Map(token ? [["aowugong_token", token]] : []);
  const requests = [];
  const window = {
    localStorage: {
      getItem: (key) => values.get(key) ?? null,
      setItem: (key, value) => values.set(key, value),
      removeItem: (key) => values.delete(key),
    },
    location: { href: "/current" },
  };
  const fetch = async (input, init = {}) => {
    requests.push({ input, init });
    return new Response("{}", { status, headers: { "Content-Type": "application/json" } });
  };
  const module = { exports: {} };
  vm.runInNewContext(output, { module, exports: module.exports, window, fetch, Headers, Response, URLSearchParams });
  return { auth: module.exports, requests, values, window };
}

test("authorizedFetch 统一注入 Bearer 并保留调用方请求头", async () => {
  const { auth, requests } = loadAuthModule();

  await auth.authorizedFetch("/api/test", { headers: { "Content-Type": "application/json" } });

  assert.equal(requests.length, 1);
  const headers = new Headers(requests[0].init.headers);
  assert.equal(headers.get("Authorization"), "Bearer test-token");
  assert.equal(headers.get("Content-Type"), "application/json");
});

test("authorizedFetch 遇到 401 时清理 token 并回到登录页", async () => {
  const { auth, values, window } = loadAuthModule({ status: 401 });

  await auth.authorizedFetch("/api/test");

  assert.equal(values.has("aowugong_token"), false);
  assert.equal(window.location.href, "/login");
});

test("authorizedFetch 未登录时不发送请求", async () => {
  const { auth, requests } = loadAuthModule({ token: "" });

  await assert.rejects(auth.authorizedFetch("/api/test"), /未登录/);
  assert.equal(requests.length, 0);
});
