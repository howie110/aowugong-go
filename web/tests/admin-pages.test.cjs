const test = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const settle = () => new Promise((resolve) => setImmediate(resolve));

// 只替换页面 hooks 和 API 边界，子组件仍走真实实现。
function createRuntime() {
  const values = [];
  const effects = [];
  let cursor = 0;
  let dirty = false;

  const hooks = {
    ...React,
    useState(initial) {
      const slot = cursor++;
      if (!(slot in values)) values[slot] = typeof initial === "function" ? initial() : initial;
      return [values[slot], (next) => {
        const value = typeof next === "function" ? next(values[slot]) : next;
        if (!Object.is(value, values[slot])) {
          values[slot] = value;
          dirty = true;
        }
      }];
    },
    useMemo(compute) {
      cursor++;
      return compute();
    },
    useEffect(effect, deps) {
      const slot = cursor++;
      const previous = effects[slot];
      const changed = !previous || !deps || !previous.deps || deps.length !== previous.deps.length
        || deps.some((value, index) => !Object.is(value, previous.deps[index]));
      effects[slot] = { effect, deps };
      if (changed) {
        effect();
      }
    },
  };

  return {
    hooks,
    setDirty() { dirty = true; },
    async mount(render) {
      let tree;
      for (let attempt = 0; attempt < 30; attempt += 1) {
        dirty = false;
        cursor = 0;
        tree = render();
        await settle();
        if (!dirty) break;
      }
      return tree;
    },
    resetCursor() { cursor = 0; },
  };
}

function elements(tree) {
  if (!React.isValidElement(tree)) return [];
  return [tree, ...React.Children.toArray(tree.props.children).flatMap(elements)];
}

function text(tree) {
  if (typeof tree === "string") return tree;
  if (!React.isValidElement(tree)) return "";
  return React.Children.toArray(tree.props.children).map(text).join("").trim();
}

function button(tree, label) {
  const found = elements(tree).find((element) => typeof element.props.onClick === "function" && text(element) === label);
  assert.ok(found, `missing button ${label}`);
  return found;
}

function notices() {
  const calls = [];
  const notify = Object.fromEntries(
    ["success", "warning", "errorFrom"].map((name) => [name, (...args) => calls.push([name, ...args])]),
  );
  return { notify, calls };
}

async function loadPermissionsPage(overrides = {}) {
  const runtime = createRuntime();
  const { notify, calls: noticeCalls } = notices();
  const calls = [];
  const users = overrides.users ?? [{ id: 1, username: "admin", email: "admin@example.com", roles: ["admin"], permissions: [] }];
  const roles = overrides.roles ?? [
    { code: "admin", name: "管理员" },
    { code: "investor", name: "投资者" },
    { code: "viewer", name: "访客" },
  ];
  const api = {
    fetchPermissionUsers: async () => users.map((user) => ({ ...user })),
    fetchPermissionRoles: async () => roles.map((role) => ({ ...role })),
    addRoleToUser: async (userId, roleCode) => {
      calls.push([userId, roleCode]);
      if (overrides.fail) throw new Error("role failed");
      return { ...users[0], id: userId, roles: [...users[0].roles, roleCode] };
    },
  };
  const { PermissionsPage } = loadTypeScript("pages/permissions.tsx", {
    moduleMocks: { "pages/permissions.tsx": { react: runtime.hooks } },
    mocks: { "@/lib/permissions": api, "@/lib/notify": { notify } },
  });
  const tree = await runtime.mount(() => PermissionsPage());
  return { tree, calls, noticeCalls, runtime, render: () => { runtime.resetCursor(); return PermissionsPage(); } };
}

async function loadDatabasePage(overrides = {}) {
  const runtime = createRuntime();
  const { notify, calls: noticeCalls } = notices();
  const calls = [];
  const summary = overrides.summary ?? {
    engine: "PostgreSQL", journal_mode: "WAL", size_bytes: 2 * 1024 * 1024,
    table_count: 2, total_rows: 3,
    tables: [
      { name: "empty_table", row_count: 0, column_count: 2 },
      { name: "users", row_count: 3, column_count: 3 },
    ],
  };
  const page = {
    table: "users", total: overrides.total ?? 3, page: 1, page_size: 50,
    columns: [
      { name: "id", type: "bigint", not_null: true, primary_key: true, sensitive: false },
      { name: "secret", type: "text", not_null: false, primary_key: false, sensitive: true },
      { name: "note", type: "text", not_null: false, primary_key: false, sensitive: false },
    ],
    rows: [{ id: 1, secret: "s3cr3t", note: null }],
  };
  const api = {
    fetchDatabaseSummary: async () => summary,
    fetchDatabaseRows: async (table, currentPage, pageSize, search) => {
      calls.push(["rows", table, currentPage, pageSize, search]);
      return { ...page, table, page: currentPage, page_size: pageSize, total: overrides.total ?? 3 };
    },
    downloadDatabaseTable: async (table, search) => {
      calls.push(["export", table, search]);
      if (overrides.failExport) throw new Error("export failed");
    },
  };
  const { DatabasePage } = loadTypeScript("pages/database.tsx", {
    moduleMocks: { "pages/database.tsx": { react: runtime.hooks } },
    mocks: { "@/lib/database": api, "@/lib/notify": { notify } },
    globals: {
      window: {
        setTimeout(callback) { callback(); return 0; },
        clearTimeout() {},
      },
    },
  });
  const tree = await runtime.mount(() => DatabasePage());
  return { tree, calls, noticeCalls, runtime, render: () => { runtime.resetCursor(); return DatabasePage(); } };
}

test("permissions page assigns the first missing role and advances the selection", async () => {
  const page = await loadPermissionsPage();
  const html = renderToStaticMarkup(page.tree);
  assert.ok(html.includes("admin@example.com"));
  assert.ok(html.includes("管理员"));

  await button(page.tree, "加入").props.onClick();
  await settle();
  assert.deepEqual(page.calls, [[1, "investor"]]);
  assert.ok(page.noticeCalls.some(([kind, title, body]) => kind === "success" && title === "角色已加入" && body.includes("投资者")));

  await button(page.render(), "加入").props.onClick();
  await settle();
  assert.deepEqual(page.calls, [[1, "investor"], [1, "viewer"]]);
});

test("permissions page reports failures and keeps controls usable", async () => {
  const page = await loadPermissionsPage({ fail: true });
  await button(page.tree, "加入").props.onClick();
  await settle();
  assert.ok(page.noticeCalls.some(([kind, error]) => kind === "errorFrom" && error.message === "role failed"));
  assert.equal(button(page.render(), "加入").props.disabled, false);
});

test("permissions page warns without a role instead of calling the API", async () => {
  const page = await loadPermissionsPage({ roles: [] });
  await button(page.tree, "加入").props.onClick();
  await settle();
  assert.deepEqual(page.calls, []);
  assert.ok(page.noticeCalls.some(([kind, message]) => kind === "warning" && message === "请选择角色"));
});

test("database page selects the first non-empty table and masks sensitive values", async () => {
  const page = await loadDatabasePage();
  const html = renderToStaticMarkup(page.tree);
  assert.equal(page.calls[0][1], "users");
  assert.ok(html.includes("users"));
  assert.ok(html.includes("已隐藏"));
  assert.ok(html.includes("NULL"));
  assert.ok(!html.includes("s3cr3t"));

  await button(page.tree, "导出").props.onClick();
  await settle();
  assert.deepEqual(page.calls.at(-1), ["export", "users", ""]);
});

test("database page debounces search, resets the page, and pages and exports with the filter", async () => {
  const page = await loadDatabasePage({ total: 120 });
  const input = elements(page.tree).find((element) => element.props?.["aria-label"] === "搜索当前表");
  assert.ok(input, "missing search input");
  input.props.onChange({ target: { value: "alice" } });
  const tree = await page.runtime.mount(() => page.render());

  assert.deepEqual(page.calls.at(-1), ["rows", "users", 1, 50, "alice"]);
  const next = elements(tree).find((element) => element.type?.displayName === "PaginationNext");
  assert.ok(next, "missing next page button");
  assert.equal(next.props.disabled, false);
  next.props.onClick();
  const paged = await page.runtime.mount(() => page.render());
  assert.deepEqual(page.calls.at(-1), ["rows", "users", 2, 50, "alice"]);

  await button(paged, "导出").props.onClick();
  await settle();
  assert.deepEqual(page.calls.at(-1), ["export", "users", "alice"]);
});
