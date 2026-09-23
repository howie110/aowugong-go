const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("../../node_modules/typescript");

// 加载 TS/TSX，并在模块边界注入可观察的网络和浏览器依赖。
function loadTypeScript(entry, { mocks = {}, globals = {}, moduleMocks = {} } = {}) {
  const root = path.resolve(__dirname, "../../src");
  const cache = new Map();
  function load(filename) {
    const resolved = [filename, `${filename}.ts`, `${filename}.tsx`].find((p) => fs.existsSync(p) && fs.statSync(p).isFile());
    if (!resolved) throw new Error(`Module not found: ${filename}`);
    if (cache.has(resolved)) return cache.get(resolved).exports;
    const module = { exports: {} };
    cache.set(resolved, module);
    const source = ts.transpileModule(fs.readFileSync(resolved, "utf8"), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true },
    }).outputText;
    const localRequire = (specifier) => {
      const overrides = moduleMocks[path.relative(root, resolved)] || {};
      if (Object.hasOwn(overrides, specifier)) return overrides[specifier];
      if (Object.hasOwn(mocks, specifier)) return mocks[specifier];
      if (specifier.startsWith("@/")) return load(path.join(root, specifier.slice(2)));
      if (specifier.startsWith(".")) return load(path.resolve(path.dirname(resolved), specifier));
      return require(specifier);
    };
    vm.runInNewContext(source, { module, exports: module.exports, require: localRequire, console,
      Error, Headers, Response, Request, URL, URLSearchParams, FormData, Blob, setTimeout, clearTimeout, ...globals,
    }, { filename: resolved });
    return module.exports;
  }
  return load(path.resolve(root, entry));
}
module.exports = { loadTypeScript };
