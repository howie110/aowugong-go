const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const pageSource = fs.readFileSync(path.resolve(__dirname, "../src/pages/vpn.tsx"), "utf8");
const libSource = fs.readFileSync(path.resolve(__dirname, "../src/lib/vpn.ts"), "utf8");

test("VPN 分配页只读展示公共分流规则", () => {
	assert.match(pageSource, /公共分流规则/);
	assert.match(pageSource, /只读/);
	assert.match(pageSource, /common_routing/);
	assert.match(pageSource, /<pre/);
	assert.match(pageSource, /handleCopyCommonRouting[\s\S]*copyTextToClipboard\(body\)/);
	assert.doesNotMatch(pageSource, /saveCommonRouting|updateCommonRouting|\/common-routing/);
});

test("VPN 资源页不展示独立的 v2rayN 分流规则资源", () => {
	assert.doesNotMatch(libSource, /routing_url/);
	assert.doesNotMatch(pageSource, /v2rayN 分流规则/);
	assert.doesNotMatch(pageSource, /subscription\.routing_url/);
	assert.doesNotMatch(pageSource, /公共分流规则需要在 v2rayN 中单独导入/);
});

test("VPN 管理页允许给已有用户追加其他资源", () => {
	assert.doesNotMatch(pageSource, /users\.filter\(\(user\) => !user\.has_subscription\)\.map/);
});
