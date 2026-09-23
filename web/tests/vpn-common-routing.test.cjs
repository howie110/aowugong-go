const assert = require("node:assert/strict");
const test = require("node:test");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { loadTypeScript } = require("./helpers/load-typescript.cjs");

const routing = { filename: "common-routing.json", body: '{"domain":["example.com"]}' };
const subscription = { id: 1, user_id: 2, username: "用户", profile_code: "dmit", status: "active", token_version: 1, subscriptions: { surge: "https://example.test/sub" } };
const summary = {
  can_manage: true, distributor_configured: true, distributor_url: "https://example.test",
  profiles: [{ code: "dmit", name: "DMIT", formats: [{ code: "surge", name: "Surge" }] }],
  users: [{ id: 2, username: "用户", email: "", has_subscription: true }],
  user_subscriptions: [subscription], common_routing: routing,
};

function loadPage(name, data) {
  let state = 0;
  const copies = [];
  const module = loadTypeScript(`pages/vpn/${name}-page.tsx`, {
    moduleMocks: { [`pages/vpn/${name}-page.tsx`]: { react: {
      ...React,
      useState(initial) { const index = state++; return [index === 0 ? data : index === 1 ? false : initial, () => {}]; },
      useEffect() {}, useMemo: (compute) => compute(),
    } } },
    mocks: { "@/lib/clipboard": { copyTextToClipboard: async (value) => { copies.push(value); return true; } },
      "@/lib/notify": { notify: { success() {}, warning() {}, error() {}, errorFrom() {} } } },
  });
  return { Page: module[name === "resources" ? "VPNResourcesPage" : "VPNDistributionPage"], copies };
}

function findElement(element, predicate) {
  if (!element || typeof element !== "object") return null;
  if (predicate(element)) return element;
  for (const child of React.Children.toArray(element.props?.children)) {
    const found = findElement(child, predicate);
    if (found) return found;
  }
  return null;
}

test("用户页展示公共规则和客户端入口，无独立规则资源或编辑入口", async () => {
  const { Page, copies } = loadPage("resources", summary);
  const tree = Page();
  const html = renderToStaticMarkup(tree);
  assert.match(html, /公共分流规则/);
  assert.match(html, /example.com/);
  assert.match(html, /扫码配置/);
  assert.match(html, /复制链接/);
  assert.doesNotMatch(html, /<textarea|复制规则|保存规则|v2rayN 分流规则/);
  const copy = findElement(tree, (item) => item.props?.children?.some?.((child) => child === "复制链接"));
  assert.ok(copy);
  copy.props.onClick();
  assert.equal(copies[0], subscription.subscriptions.surge);
});

test("管理员页面只读展示规则并复制原文", () => {
  const { Page, copies } = loadPage("distribution", summary);
  const tree = Page();
  const html = renderToStaticMarkup(tree);
  assert.match(html, /公共分流规则/);
  assert.match(html, /只读/);
  assert.match(html, /复制规则/);
  assert.doesNotMatch(html, /<textarea|保存规则/);
  const card = findElement(tree, (item) => typeof item.props?.onCopy === "function");
  assert.ok(card);
  card.props.onCopy();
  assert.equal(copies[0], routing.body);
});

test("空资源和缺失规则仍显示明确占位", () => {
  const { Page } = loadPage("resources", { ...summary, user_subscriptions: [], common_routing: undefined });
  const html = renderToStaticMarkup(Page());
  assert.match(html, /尚未分配 VPN 资源/);
  assert.match(html, /暂无公共规则配置/);
});

test("已有资源的用户仍能出现在开通用户选项中", () => {
  // 用 HTML 替身展开 portal 控件，验证业务组件生成的选项。
  const passthrough = ({ children }) => React.createElement("div", null, children);
  const { CreateUserDialog } = loadTypeScript("pages/vpn/create-user-dialog.tsx", { mocks: {
    "@/components/ui/dialog": Object.fromEntries(["Dialog", "DialogContent", "DialogHeader", "DialogTitle", "DialogDescription", "DialogFooter"].map((key) => [key, passthrough])),
    "@/components/ui/select": Object.fromEntries(["Select", "SelectTrigger", "SelectValue", "SelectContent", "SelectItem"].map((key) => [key, passthrough])),
  } });
  const html = renderToStaticMarkup(React.createElement(CreateUserDialog, {
    open: true, userID: "", profileCode: "", users: summary.users, profiles: summary.profiles, isSaving: false,
    onOpenChange() {}, onUserChange() {}, onProfileChange() {}, onSave() {},
  }));
  assert.match(html, /用户 ·/);
  assert.match(html, /DMIT/);
});
