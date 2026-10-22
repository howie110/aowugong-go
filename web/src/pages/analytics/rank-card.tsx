import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { Rank } from "@/lib/analytics";
import { number } from "./charts";
export const labels: Record<string, string> = {
  path: "路径",
  url: "URL",
  entry: "入口页",
  exit: "退出页",
  referrer: "来源域名",
  channel: "渠道",
  browser: "浏览器",
  os: "操作系统",
  device: "设备",
  language: "语言",
  country: "国家/地区",
  region: "省/州",
  city: "城市",
};
const translations: Record<string, string> = {
  direct: "直接访问",
  search: "搜索",
  social: "社交",
  referral: "引荐",
  email: "邮件",
  paid: "付费",
  unknown: "未知",
  desktop: "电脑",
  mobile: "手机",
  tablet: "平板",
};
export function displayName(key: string, value: string) {
  if (!value) return key === "referrer" ? "直接访问" : "未知";
  if (key === "channel" || key === "device")
    return translations[value] || value;
  if (key === "country") {
    try {
      return (
        new Intl.DisplayNames(["zh-CN"], { type: "region" }).of(value) || value
      );
    } catch {
      return value;
    }
  }
  return value;
}
export function RankCard({
  title,
  tabs,
  ranks,
  onFilter,
}: {
  title: string;
  tabs: string[];
  ranks: Record<string, Rank[]>;
  onFilter: (k: string, v: string) => void;
}) {
  const [active, setActive] = useState(tabs[0]);
  const items = ranks[active] || [];
  const max = Math.max(1, ...items.map((r) => r.count));
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">{title}</CardTitle>
        <div
          className="flex flex-wrap gap-1 pt-2"
          role="tablist"
          aria-label={title}
        >
          {tabs.map((key) => (
            <Button
              key={key}
              role="tab"
              aria-selected={active === key}
              variant={key === active ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActive(key)}
            >
              {labels[key]}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent>
        <div className="mb-2 flex justify-between text-xs text-muted-foreground">
          <span>点击条目筛选</span>
          <span>
            {active === "entry" || active === "exit" ? "访问次数" : "浏览量"} ·{" "}
            {active === "country" ? "全部地区" : "前 100 项"}
          </span>
        </div>
        <div className="h-64 overflow-auto" role="tabpanel">
          {items.length === 0 ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              暂无数据
            </div>
          ) : (
            items.map((r) => (
              <button
                key={r.name}
                className="relative mb-1 flex w-full items-center justify-between gap-3 overflow-hidden rounded px-3 py-2 text-left text-sm hover:bg-muted"
                onClick={() => onFilter(active, r.name)}
                title={displayName(active, r.name)}
              >
                <span
                  className="absolute inset-y-0 left-0 bg-blue-500/10"
                  style={{ width: `${(100 * r.count) / max}%` }}
                />
                <span className="relative min-w-0 truncate">
                  {displayName(active, r.name)}
                </span>
                <span className="relative tabular-nums">{number(r.count)}</span>
              </button>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  );
}
