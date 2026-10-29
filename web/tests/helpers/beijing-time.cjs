const { loadTypeScript } = require("./load-typescript.cjs");

// 测试注入直接使用生产实现，避免时间逻辑出现第二份副本。
module.exports = loadTypeScript("lib/beijing-time.ts");
