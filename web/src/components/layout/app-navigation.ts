import {
  BarChart3,
  Bell,
  BookOpen,
  CalendarClock,
  Compass,
  Database,
  CreditCard,
  LineChart,
  Newspaper,
  PlayCircle,
  Radar,
  Rss,
  Share2,
  ShieldCheck,
  Trophy,
  TrendingUp,
  TableProperties,
  UserCog,
  WalletCards,
  Wifi,
  type LucideIcon,
} from "lucide-react";

import { type FinancePageKey, pagePermissionMap } from "@/lib/finance";

export type NavItem = {
  key: FinancePageKey;
  label: string;
  icon: LucideIcon;
  permission: string;
};

export type NavGroup = {
  id: string;
  label: string;
  icon: LucideIcon;
  items: NavItem[];
};

export const NAV_GROUP_STORAGE_KEY = "aowugong.sidebar.openGroups";

export const navGroups: NavGroup[] = [
  {
    id: "general",
    label: "总览入口",
    icon: BarChart3,
    items: [
      { key: "overview", label: "控制台", icon: BarChart3, permission: pagePermissionMap.overview },
      { key: "work", label: "工作导航", icon: Compass, permission: pagePermissionMap.work },
    ],
  },
  {
    id: "investment",
    label: "投资研究",
    icon: TrendingUp,
    items: [
      { key: "articleAnalysis", label: "投资文章分析", icon: Newspaper, permission: pagePermissionMap.articleAnalysis },
      { key: "articleFetch", label: "投资文章抓取", icon: Rss, permission: pagePermissionMap.articleFetch },
      { key: "stockAnalysis", label: "股票仓位分析", icon: TrendingUp, permission: pagePermissionMap.stockAnalysis },
      { key: "positions", label: "股票仓位导入", icon: WalletCards, permission: pagePermissionMap.positions },
    ],
  },
  {
    id: "quant",
    label: "量化工具",
    icon: LineChart,
    items: [
      { key: "backtest", label: "回测", icon: LineChart, permission: pagePermissionMap.backtest },
      { key: "data", label: "数据", icon: Database, permission: pagePermissionMap.data },
      { key: "trading", label: "交易", icon: PlayCircle, permission: pagePermissionMap.trading },
    ],
  },
  {
    id: "content",
    label: "内容服务",
    icon: BookOpen,
    items: [
      { key: "weread", label: "微信读书", icon: BookOpen, permission: pagePermissionMap.weread },
      { key: "mahjong", label: "麻将战绩", icon: Trophy, permission: pagePermissionMap.mahjong },
      { key: "subscriptions", label: "订阅管理", icon: CreditCard, permission: pagePermissionMap.subscriptions },
    ],
  },
  {
    id: "resource-sharing",
    label: "资源分享",
    icon: Share2,
    items: [
      { key: "vpnDistribution", label: "VPN 分配", icon: UserCog, permission: pagePermissionMap.vpnDistribution },
      { key: "vpnResources", label: "VPN 资源", icon: Wifi, permission: pagePermissionMap.vpnResources },
    ],
  },
  {
    id: "system",
    label: "系统运维",
    icon: ShieldCheck,
    items: [
      { key: "monitoring", label: "监控管理", icon: Radar, permission: pagePermissionMap.monitoring },
      { key: "jobs", label: "定时任务", icon: CalendarClock, permission: pagePermissionMap.jobs },
      { key: "notifications", label: "通知", icon: Bell, permission: pagePermissionMap.notifications },
      { key: "database", label: "数据库", icon: TableProperties, permission: pagePermissionMap.database },
      { key: "permissions", label: "权限管理", icon: UserCog, permission: pagePermissionMap.permissions },
    ],
  },
];

export const navItems = navGroups.flatMap((group) => group.items);
export const pageLabelMap = Object.fromEntries(navItems.map((item) => [item.key, item.label])) as Record<FinancePageKey, string>;
export const pageGroupMap = Object.fromEntries(
  navGroups.flatMap((group) => group.items.map((item) => [item.key, group.id])),
) as Record<FinancePageKey, string>;
export const pageGroupLabelMap = Object.fromEntries(
  navGroups.flatMap((group) => group.items.map((item) => [item.key, group.label])),
) as Record<FinancePageKey, string>;
