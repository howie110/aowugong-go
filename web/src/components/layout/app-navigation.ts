import {
  MapPin,
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

import type { FinancePageKey } from "@/lib/finance";

export type NavItem = {
  key: FinancePageKey;
  label: string;
  icon: LucideIcon;
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
      { key: "overview", label: "控制台", icon: BarChart3 },
      { key: "work", label: "工作导航", icon: Compass },
    ],
  },
  {
    id: "investment",
    label: "投资研究",
    icon: TrendingUp,
    items: [
      { key: "articleAnalysis", label: "投资文章分析", icon: Newspaper },
      { key: "articleFetch", label: "投资文章抓取", icon: Rss },
      { key: "stockAnalysis", label: "股票仓位分析", icon: TrendingUp },
      { key: "positions", label: "股票仓位导入", icon: WalletCards },
    ],
  },
  {
    id: "quant",
    label: "量化工具",
    icon: LineChart,
    items: [
      { key: "backtest", label: "回测", icon: LineChart },
      { key: "data", label: "数据", icon: Database },
      { key: "trading", label: "交易", icon: PlayCircle },
    ],
  },
  {
    id: "content",
    label: "内容服务",
    icon: BookOpen,
    items: [
      { key: "blogStatus", label: "记录此刻", icon: Newspaper },
      { key: "weread", label: "微信读书", icon: BookOpen },
      { key: "mahjong", label: "麻将战绩", icon: Trophy },
      { key: "subscriptions", label: "订阅管理", icon: CreditCard },
      { key: "chatArchive", label: "聊天归档", icon: BookOpen },
    ],
  },
  {
    id: "resource-sharing",
    label: "资源分享",
    icon: Share2,
    items: [
      { key: "vpnDistribution", label: "VPN 分配", icon: UserCog },
      { key: "vpnResources", label: "VPN 资源", icon: Wifi },
    ],
  },
  {
    id: "personal",
    label: "个人工具",
    icon: MapPin,
    items: [{key:"trajectory",label:"我的轨迹",icon:MapPin}],
  },
  {
    id: "website",
    label: "网站运维",
    icon: BarChart3,
    items: [{ key: "analytics", label: "访问统计", icon: BarChart3 }, { key:"receiptSplit",label:"小票分账",icon:TableProperties }],
  },
  {
    id: "system",
    label: "系统运维",
    icon: ShieldCheck,
    items: [
      { key: "monitoring", label: "监控管理", icon: Radar },
      { key: "jobs", label: "定时任务", icon: CalendarClock },
      { key: "notifications", label: "通知", icon: Bell },
      { key: "database", label: "数据库", icon: TableProperties },
      { key: "permissions", label: "权限管理", icon: UserCog },
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
