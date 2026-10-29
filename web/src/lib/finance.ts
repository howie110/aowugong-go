import { requestJSON } from "@/lib/request";
import type { WeReadDashboardData } from "@/lib/weread";

export type FinancePageKey =
  | "overview"
  | "blogStatus"
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
  | "trajectory"
  | "receiptSplit"
  | "analytics"
  | "chatArchive"
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
  blogStatus: "blog.status.manage",
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
  trajectory: "page:personal:trajectory",
  receiptSplit: "page:website:receipt_split",
  analytics: "page:website:analytics",
  chatArchive: "page:chat_archive",
  monitoring: "page:monitoring",
  work: "page:work",
  database: "page:database",
  vpnDistribution: "page:resource_sharing:vpn_distribution",
  vpnResources: "page:resource_sharing:vpn_resources",
  permissions: "page:permissions",
};

export const pageMetaMap: Record<FinancePageKey, { title: string; description: string }> = {
  trajectory: { title: "我的轨迹", description: "保存自己的位置记录，查看采集与补传情况。" },
  receiptSplit: { title:"小票分账", description:"查看小票分账用户、识别任务和账单使用趋势。" },
  analytics: { title: "访问统计", description: "查看全站访问趋势、页面与来源分布。" },
  chatArchive: { title: "聊天归档", description: "核对消息数量、时间与同步记录。" },
  blogStatus: { title: "记录此刻", description: "发布文字与图片，记录生活片刻。" },
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
    description: "管理信息源，抓取 RSS 文章，并触发结构化模型分析。",
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
    description: "查看当前账号获配的 VPN 资源，扫码或复制订阅链接配置客户端。",
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
  trajectory: "/api/v1/trajectory/points",
  receiptSplit: "/api/v1/receipt-split/statistics",
  analytics: "/api/v1/analytics/report/",
  chatArchive: "/api/v1/chat-archive/conversations",
 blogStatus: "/api/v1/blog/admin/statuses",
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

// 页面路径只在此维护；保留已有 URL 和未知路径的控制台回退。
const pagePaths: Record<FinancePageKey, string> = {
  overview: "/work",
  blogStatus: "/work/content/status",
  weread: "/weread",
  positions: "/positions",
  stockAnalysis: "/stock-analysis",
  articleFetch: "/article-fetch",
  articleAnalysis: "/article-analysis",
  backtest: "/backtest",
  data: "/data",
  jobs: "/jobs",
  trading: "/trading",
  notifications: "/notifications",
  mahjong: "/mahjong",
  subscriptions: "/subscriptions",
  trajectory: "/work/personal/trajectory",
  receiptSplit: "/work/website/receipt-split",
  analytics: "/work/website/analytics",
  chatArchive: "/work/content/chat-archive",
  monitoring: "/monitoring",
  work: "/work/navigation",
  database: "/database",
  vpnDistribution: "/vpn-distribution",
  vpnResources: "/vpn-resources",
  permissions: "/permissions",
};
const pagesByPath = new Map(Object.entries(pagePaths).map(([key, path]) => [path.slice(1), key as FinancePageKey]));

export function getFinancePagePath(pageKey: FinancePageKey) {
  return pagePaths[pageKey];
}

export function getFinancePageFromPath(pathname: string): FinancePageKey {
  return pagesByPath.get(pathname.replace(/^\/+|\/+$/g, "")) ?? "overview";
}

export { ApiError } from "@/lib/request";

export async function fetchFinancePage(pageKey: FinancePageKey): Promise<FinancePageData> {
  return requestJSON<FinancePageData>(pageEndpointMap[pageKey], {}, "获取页面数据失败");
}

export type NotificationLogEntry = {
  id: number;
  channel: string;
  title: string;
  message: string;
  status: string;
  error_message: string;
  sent_at: string;
  source: string;
  request_id: string;
};

export type NotificationLogPage = {
  items: NotificationLogEntry[];
  total: number;
  page: number;
  page_size: number;
};

export type NotificationLogFilters = {
  q?: string;
  status?: string;
  category?: string;
  page?: number;
  pageSize?: number;
};

/** fetchNotificationLogs 读取微信发送历史的分页和筛选结果。 */
export function fetchNotificationLogs(filters: NotificationLogFilters = {}) {
  // 1. 只把非空筛选拼入查询，分页参数始终显式传递。
  const query = new URLSearchParams();
  if (filters.q?.trim()) {
    query.set("q", filters.q.trim());
  }
  if (filters.status) {
    query.set("status", filters.status);
  }
  if (filters.category) {
    query.set("category", filters.category);
  }
  query.set("page", String(filters.page ?? 1));
  query.set("page_size", String(filters.pageSize ?? 20));
  return requestJSON<NotificationLogPage>(
    `/api/v1/finance/notifications/logs?${query.toString()}`, {}, "读取通知历史失败",
  );
}

export type NotificationExternalConfig = {
  endpoint: string;
  project: string;
  configured: boolean;
  token: string;
  max_message_bytes: number;
  max_request_bytes: number;
  rate_limit_per_minute: number;
};

/** fetchNotificationExternalConfig 读取外部服务调用通知 API 的固定配置。 */
export function fetchNotificationExternalConfig() {
  // 1. 该接口只对通知页权限开放，返回 external 项目 Token 和限制。
  return requestJSON<NotificationExternalConfig>(
    "/api/v1/finance/notifications/external", {}, "读取外部调用配置失败",
  );
}
