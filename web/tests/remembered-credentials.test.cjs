const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("../node_modules/typescript");

const modulePath = path.resolve(__dirname, "../src/lib/remembered-credentials.ts");

/** 编译并加载待测试的 TypeScript 凭据模块。 */
function loadModule() {
  assert.equal(fs.existsSync(modulePath), true, "缺少记住密码存取模块");

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

/** 创建满足浏览器存储接口的内存测试对象。 */
function createStorage(initialValue) {
  const values = new Map(initialValue ? [["aowugong_remembered_credentials", initialValue]] : []);

  return {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      values.set(key, value);
    },
    removeItem(key) {
      values.delete(key);
    },
  };
}

/** 创建所有存取操作都会失败的浏览器存储对象。 */
function createThrowingStorage() {
  return {
    getItem() {
      throw new Error("storage unavailable");
    },
    setItem() {
      throw new Error("storage full");
    },
    removeItem() {
      throw new Error("storage unavailable");
    },
  };
}

test("没有保存记录时返回空值", () => {
  const { getRememberedCredentials } = loadModule();

  assert.equal(getRememberedCredentials(createStorage()), null);
});

test("保存后可以恢复用户名和密码", () => {
  const { getRememberedCredentials, saveRememberedCredentials } = loadModule();
  const storage = createStorage();

  saveRememberedCredentials(storage, { username: "demo", password: "demo-password" });

  assert.deepEqual(
    { ...getRememberedCredentials(storage) },
    { username: "demo", password: "demo-password" },
  );
});

test("取消记住密码后清除保存记录", () => {
  const { clearRememberedCredentials, getRememberedCredentials, saveRememberedCredentials } = loadModule();
  const storage = createStorage();
  saveRememberedCredentials(storage, { username: "demo", password: "demo-password" });

  clearRememberedCredentials(storage);

  assert.equal(getRememberedCredentials(storage), null);
});

test("损坏的浏览器记录会被忽略并清除", () => {
  const { getRememberedCredentials } = loadModule();
  const storage = createStorage("not-json");

  assert.equal(getRememberedCredentials(storage), null);
  assert.equal(storage.getItem("aowugong_remembered_credentials"), null);
});

test("浏览器拒绝读取存储时返回空值", () => {
  const { getRememberedCredentials } = loadModule();

  assert.equal(getRememberedCredentials(createThrowingStorage()), null);
});

test("浏览器拒绝提供本地存储对象时返回空值", () => {
  const { getCredentialStorage } = loadModule();
  const browserWindow = {
    get localStorage() {
      throw new Error("localStorage blocked");
    },
  };

  assert.equal(getCredentialStorage(browserWindow), null);
});

test("浏览器拒绝写入或清除存储时不影响登录流程", () => {
  const { clearRememberedCredentials, saveRememberedCredentials } = loadModule();
  const storage = createThrowingStorage();

  assert.doesNotThrow(() => saveRememberedCredentials(storage, { username: "demo", password: "demo-password" }));
  assert.doesNotThrow(() => clearRememberedCredentials(storage));
});
