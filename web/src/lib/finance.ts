import { authorizedFetch } from "@/lib/auth";
import type { WeReadDashboardData } from "@/lib/weread";

export type FinancePageKey =
  | "overview"
  | "weread"
  | "positions"
  | "stockAnalysis"
  | "articleFetch"
  | "articleAnalysis"
  | "backtest"
  | "data"
  | "jobs"
  | "trading"
  | "notifications"
  | "mahjong"
  | "subscriptions"
  | "monitoring"
  | "work"
  | "database"
  | "vpnDistribution"
  | "vpnResources"
  | "permissions";

export type FinanceMetric = {
  label: string;
  value: string;
  detail?: string;
  status?: string;
};

export type FinanceItem = {
  name: string;
  description?: string;
  latest?: string;
  schedule?: string;
  status?: string;
  value?: string;
  command?: string;
  entry?: string;
  date_column?: string;
};

export type FinancePageData = {
  title: string;
  description: string;
  metrics?: FinanceMetric[];
  modules?: FinanceItem[];
  data_progress?: FinanceItem[];
  items?: FinanceItem[];
  tables?: FinanceItem[];
  sources?: FinanceItem[];
  jobs?: FinanceItem[];
  guards?: FinanceItem[];
  channels?: FinanceItem[];
  runner?: string;
  fail_notify?: string;
  receiver_count?: number;
  weread?: WeReadDashboardData;
};

export const pagePermissionMap: Record<FinancePageKey, string> = {
  overview: "page:finance:overview",
  weread: "page:weread",
  positions: "page:finance:positions",
  stockAnalysis: "page:finance:stock_analysis",
  articleFetch: "page:finance:article_fetch",
  articleAnalysis: "page:finance:article_analysis",
  backtest: "page:finance:backtest",
  data: "page:finance:data",
  jobs: "page:finance:jobs",
  trading: "page:finance:trading",
  notifications: "page:finance:notifications",
  mahjong: "page:mahjong",
  subscriptions: "page:subscriptions",
  monitoring: "page:monitoring",
  work: "page:work",
  database: "page:database",
  vpnDistribution: "page:resource_sharing:vpn_distribution",
  vpnResources: "page:resource_sharing:vpn_resources",
  permissions: "page:permissions",
};

export const pageMetaMap: Record<FinancePageKey, { title: string; description: string }> = {
  overview: {
    title: "控制台",
    description: "集中查看投资研究、内容服务、定时任务和系统运维状态。",
  },
  weread: {
    title: "微信读书",
    description: "实时读取微信读书账号的阅读统计和阅读进度，不在本地落库。",
  },
  positions: {
    title: "股票仓位导入",
    description: "上传同花顺仓位截图，只展示导入记录。",
  },
  stockAnalysis: {
    title: "股票仓位分析",
    description: "基于导入记录生成组合趋势、持仓分布和变化分析。",
  },
  articleAnalysis: {
    title: "投资文章分析",
    description: "统计投资文章中的推荐标的、风险标的、市场氛围和涨跌预测。",
  },
  articleFetch: {
    title: "投资文章抓取",
    description: "管理信息源，抓取 RSS 文章，并触发 DeepSeek 结构化分析。",
  },
  backtest: {
    title: "回测",
    description: "策略回测结构和统一入口。",
  },
  data: {
    title: "数据",
    description: "行情、交易日历和基础数据状态。",
  },
  jobs: {
    title: "定时任务",
    description: "Go 进程内调度和统一任务入口状态。",
  },
  trading: {
    title: "交易",
    description: "实盘交易保护和交易模块边界。",
  },
  notifications: {
    title: "通知",
    description: "失败提醒和业务消息推送渠道。",
  },
  mahjong: {
    title: "麻将战绩",
    description: "录入麻将战绩，查看累计输赢、胜率、场均和打牌频率。",
  },
  subscriptions: {
    title: "订阅管理",
    description: "记录云服务、域名和生活会员的费用、状态和到期日。",
  },
  monitoring: {
    title: "监控管理",
    description: "集中查看服务器项目、RSS、博客和通知桥接服务的连通性。",
  },
  work: {
    title: "工作导航",
    description: "常用系统、工具和资料入口。",
  },
  database: {
    title: "数据库",
    description: "只读查看 PostgreSQL 表结构、数据和脱敏导出。",
  },
  vpnDistribution: {
    title: "VPN 分配",
    description: "给登录用户分配 VPN 资源并维护订阅状态。",
  },
  vpnResources: {
    title: "VPN 资源",
    description: "查看当前账号获配的 VPN 资源并扫码配置客户端。",
  },
  permissions: {
    title: "权限管理",
    description: "把用户加入角色，角色权限由系统预设维护。",
  },
};

export function getFinancePageMeta(pageKey: FinancePageKey) {
  return pageMetaMap[pageKey];
}

const pageEndpointMap: Record<FinancePageKey, string> = {
  overview: "/api/v1/finance/overview/",
  weread: "/api/v1/weread/dashboard",
  positions: "/api/v1/finance/positions/summary",
  stockAnalysis: "/api/v1/finance/stock-analysis/summary",
  articleFetch: "/api/v1/finance/article-analysis/fetch-summary",
  articleAnalysis: "/api/v1/finance/article-analysis/summary",
  backtest: "/api/v1/finance/backtest/summary",
  data: "/api/v1/finance/data/summary",
  jobs: "/api/v1/finance/jobs/summary",
  trading: "/api/v1/finance/trading/summary",
  notifications: "/api/v1/finance/notifications/summary",
  mahjong: "/api/v1/mahjong/summary",
  subscriptions: "/api/v1/subscriptions/summary",
  monitoring: "/api/v1/monitoring/summary",
  work: "/api/v1/work/navigation",
  database: "/api/v1/database/summary",
  vpnDistribution: "/api/v1/vpn/distribution/summary",
  vpnResources: "/api/v1/vpn/resources/summary",
  permissions: "/api/v1/permissions/summary",
};

export function getFinancePagePath(pageKey: FinancePageKey) {
  if (pageKey === "overview") {
    return "/";
  }
  if (pageKey === "stockAnalysis") {
    return "/stock-analysis";
  }
  if (pageKey === "articleAnalysis") {
    return "/article-analysis";
  }
  if (pageKey === "articleFetch") {
    return "/article-fetch";
  }
  if (pageKey === "weread") {
    return "/weread";
  }
  if (pageKey === "work") {
    return "/work";
  }
  if (pageKey === "mahjong") {
    return "/mahjong";
  }
  if (pageKey === "subscriptions") {
    return "/subscriptions";
  }
  if (pageKey === "permissions") {
    return "/permissions";
  }
  if (pageKey === "monitoring") {
    return "/monitoring";
  }
  if (pageKey === "database") {
    return "/database";
  }
  if (pageKey === "vpnDistribution") {
    return "/vpn-distribution";
  }
  if (pageKey === "vpnResources") {
    return "/vpn-resources";
  }
  return `/${pageKey}`;
}

export function getFinancePageFromPath(pathname: string): FinancePageKey {
  const path = pathname.replace(/^\/+|\/+$/g, "");
  if (path === "stock-analysis") {
    return "stockAnalysis";
  }
  if (path === "article-analysis") {
    return "articleAnalysis";
  }
  if (path === "article-fetch") {
    return "articleFetch";
  }
  if (path === "vpn-distribution") {
    return "vpnDistribution";
  }
  if (path === "vpn-resources") {
    return "vpnResources";
  }
  if (
    path === "weread" ||
    path === "positions" ||
    path === "backtest" ||
    path === "data" ||
    path === "jobs" ||
    path === "trading" ||
    path === "notifications" ||
    path === "mahjong" ||
    path === "subscriptions" ||
    path === "monitoring" ||
    path === "work" ||
    path === "database" ||
    path === "permissions"
  ) {
    return path;
  }
  return "overview";
}

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export async function fetchFinancePage(pageKey: FinancePageKey): Promise<FinancePageData> {
  const response = await authorizedFetch(pageEndpointMap[pageKey]);

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new ApiError(data?.detail || "获取页面数据失败", response.status);
  }

  return (await response.json()) as FinancePageData;
}
