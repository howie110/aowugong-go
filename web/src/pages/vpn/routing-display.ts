type Rule = {
  type: string; outboundTag: string; domain?: string[]; ip?: string[];
  port?: string; network?: string;
};
export type RoutingSection = { title: string; rows: { line: string; note: string; order: number }[] };

// 用途标签仅用于展示，不生成规则；只合并相邻同组，避免改变实际匹配顺序。
function proxyGroup(value: string): string {
  const groups: [RegExp, string][] = [
    [/openai|chatgpt|oaistatic|oaiusercontent|^ai\.com$|claude|clawhub|cursor|notion/, "AI 与效率工具"],
    [/auth0|stripe|sentry|intercom|cloudflare/, "登录、支付与基础服务"],
    [/google|gstatic/, "Google 服务"],
    [/telegram|whatsapp|facebook|pinterest|pinimg|fb\.watch|instagram|tiktok/, "社交平台"],
    [/amazon|homedepot|wal-mart/, "电商与供应商平台"],
    [/github|visualstudio/, "开发工具"],
    [/3gppnetwork|usairmobile|entitled\.mobile|vowifi/, "通信与 Wi-Fi 通话"],
  ];
  return groups.find(([pattern]) => pattern.test(value.toLowerCase()))?.[1] || "其他指定网站与地址";
}

// 只格式化接口返回的规则，保持原始顺序；无法解释的配置交由页面明确展示原文。
export function describeRouting(body: string): RoutingSection[] {
  const document = JSON.parse(body);
  if (!Array.isArray(document.rules)) throw new Error("缺少规则列表");
  const sections: RoutingSection[] = [];
  let order = 0;
  for (const rule of document.rules as Rule[]) {
    const target = ({ proxy: "PROXY", direct: "DIRECT", block: "REJECT" } as Record<string, string>)[rule.outboundTag];
    if (rule.type !== "field" || !target || Object.keys(rule).some(key => !["type", "outboundTag", "domain", "ip", "port", "network"].includes(key))) throw new Error("存在暂不支持展示的规则");
    const matchers = [rule.domain?.length, rule.ip?.length, rule.port, rule.network].filter(Boolean);
    if (matchers.length !== 1) throw new Error("匹配条件不明确");
    const isDefault = rule.outboundTag === "direct" && !!rule.network;
    const title = isDefault ? "其余流量直连" : ({ proxy: "指定名单走代理", direct: "指定规则直连", block: "拦截规则" } as Record<string, string>)[rule.outboundTag];
    const add = (matcher: string, value: string, note: string) => {
      const groupTitle = rule.outboundTag === "proxy" ? `${proxyGroup(value)} · 代理` : title;
      let section = sections[sections.length - 1];
      if (!section || section.title !== groupTitle) {
        section = { title: groupTitle, rows: [] };
        sections.push(section);
      }
      section.rows.push({ line: `${matcher},${value},${target}`, note, order: ++order });
    };
    for (const value of rule.domain || []) {
      const index = value.indexOf(":");
      const kind = index < 0 ? "domain" : value.slice(0, index);
      const domain = index < 0 ? value : value.slice(index + 1);
      const spec = ({ full: ["DOMAIN", "完整域名"], domain: ["DOMAIN-SUFFIX", "含所有子域名"], keyword: ["DOMAIN-KEYWORD", "包含关键字"], regexp: ["DOMAIN-REGEX", "域名正则"] } as Record<string, string[]>)[kind];
      if (!spec || !domain) throw new Error("无法解释域名规则");
      add(spec[0], domain, spec[1]);
    }
    for (const value of rule.ip || []) {
      if (value.startsWith("geoip:")) add("GEOIP", value.slice(6).toUpperCase(), "IP 分类");
      else add(value.includes(":") ? "IP-CIDR6" : "IP-CIDR", value, "目标 IP");
    }
    if (rule.port) add("DST-PORT", rule.port, "目标端口");
    if (rule.network) add("NETWORK", rule.network.toUpperCase(), "其余流量");
  }
  return sections;
}
