const assert = require("node:assert/strict");
const test = require("node:test");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const pagePermissionMap = {
  overview: "page:finance:overview",
  trajectory: "page:personal:trajectory",
  chatArchive: "page:chat_archive",
  analytics: "page:website:analytics",
  receiptSplit: "page:website:receipt_split",
};
const { adminOnlyPages, canAccessPage } = loadTypeScript("lib/permissions.ts", {
  mocks: { "@/lib/finance": { pagePermissionMap } },
});

const admin = { id: 1, username: "admin", roles: ["admin"], permissions: [] };
const analyst = {
  id: 2,
  username: "analyst",
  roles: ["investor"],
  permissions: ["page:finance:overview", "page:personal:trajectory", "page:chat_archive"],
};
const guest = { id: 3, username: "guest", roles: ["investor"], permissions: [] };

test("管理员可以访问全部页面", () => {
  assert.deepEqual([...adminOnlyPages].sort(), ["analytics", "chatArchive", "receiptSplit", "trajectory"]);
  for (const page of ["overview", "trajectory", "chatArchive", "analytics", "receiptSplit"]) {
    assert.equal(canAccessPage(admin, page), true);
  }
});

test("普通用户只能访问已授权页面，admin-only 页面的权限码不生效", () => {
  assert.equal(canAccessPage(analyst, "overview"), true);
  assert.equal(canAccessPage(analyst, "trajectory"), false);
  assert.equal(canAccessPage(analyst, "chatArchive"), false);
  assert.equal(canAccessPage(analyst, "analytics"), false);
  assert.equal(canAccessPage(analyst, "receiptSplit"), false);
  assert.equal(canAccessPage(guest, "overview"), false);
  assert.equal(canAccessPage(null, "overview"), false);
});
