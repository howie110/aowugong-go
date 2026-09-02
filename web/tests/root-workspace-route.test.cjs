const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

function readSource(relativePath) {
  return fs.readFileSync(path.resolve(__dirname, "..", relativePath), "utf8");
}

test("根路径使用公开备案主页，工作台使用独立入口", () => {
  const mainSource = readSource("src/main.tsx");

  assert.match(mainSource, /PublicHomePage/);
  assert.match(mainSource, /path === "\/"/);
  assert.match(mainSource, /replaceState\(\{\}, "", "\/work"\)/);
});

test("工作台首页和工作导航使用不同路径", () => {
  const financeSource = readSource("src/lib/finance.ts");

  assert.match(financeSource, /pageKey === "overview"[\s\S]*?return "\/work"/);
  assert.match(financeSource, /pageKey === "work"[\s\S]*?return "\/work\/navigation"/);
  assert.match(financeSource, /path === "work"[\s\S]*?return "overview"/);
  assert.match(financeSource, /path === "work\/navigation"[\s\S]*?return "work"/);
});

test("登录成功后进入工作台，不回到公开根页面", () => {
  const loginSource = readSource("src/pages/login.tsx");

  assert.doesNotMatch(loginSource, /window\.location\.href = "\/"/);
  assert.match(loginSource, /onLoggedIn/);
});

test("备案主页提供工具分享信息和备案号位置", () => {
  const homePath = path.resolve(__dirname, "../src/pages/public-home.tsx");

  assert.equal(fs.existsSync(homePath), true);
  const homeSource = fs.readFileSync(homePath, "utf8");
  assert.match(homeSource, /工具分享/);
  assert.match(homeSource, /备案/);
});

test("备案主页使用公开网站导航布局和 shadcn 内容组件", () => {
  const homeSource = readSource("src/pages/public-home.tsx");

  assert.match(homeSource, /Badge/);
  assert.match(homeSource, /Card/);
  assert.match(homeSource, /网站导航/);
  assert.match(homeSource, /本站说明/);
  assert.match(homeSource, /publicLinkGroups/);
  assert.doesNotMatch(homeSource, /Miniflux|Nextflux|Vaultwarden|Umami/);
  assert.doesNotMatch(homeSource, /github\.com|cloudflare\.com|vercel\.com|openai\.com|google\.com/);
  assert.doesNotMatch(homeSource, /淘宝|天猫|京东|视频与购物/);
});

test("工具分享主页不导向个人博客", () => {
  const homeSource = readSource("src/pages/public-home.tsx");

  assert.doesNotMatch(homeSource, /blog\.aowugong\.top|BLOG_URL|打开博客|访问个人博客|个人博客/);
  assert.doesNotMatch(homeSource, /href="\/work"|工作台|LogIn/);
});
