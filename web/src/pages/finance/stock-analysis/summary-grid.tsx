import { ArrowDownRight, ArrowUpRight, BarChart3, WalletCards } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  formatMoney,
  formatPercent,
  formatSignedMoney,
  maskSensitive,
  sensitiveTone,
  toNumber,
} from "./format";
import type { StockAnalysisReport } from "./types";

export function SummaryGrid({ report, isSensitiveMasked }: { report: StockAnalysisReport; isSensitiveMasked: boolean }) {
  const latest = report.latest!;
  const cards = [
    {
      label: "最新总资产",
      value: maskSensitive(isSensitiveMasked, formatMoney(latest.total_asset)),
      detail: latest.snapshot_date,
      icon: WalletCards,
    },
    {
      label: "累计资产变化",
      value: maskSensitive(isSensitiveMasked, formatSignedMoney(report.changes.total_asset_change)),
      detail: "相对首个记录日，未扣除出入金",
      icon: isSensitiveMasked ? WalletCards : toNumber(report.changes.total_asset_change) >= 0 ? ArrowUpRight : ArrowDownRight,
      className: sensitiveTone(isSensitiveMasked, report.changes.total_asset_change),
    },
    {
      label: "本次资产变化",
      value: maskSensitive(isSensitiveMasked, formatSignedMoney(report.changes.daily_total_asset_change)),
      detail: report.previous ? `对比 ${report.previous.snapshot_date}` : "暂无上一记录点",
      icon: isSensitiveMasked ? WalletCards : toNumber(report.changes.daily_total_asset_change) >= 0 ? ArrowUpRight : ArrowDownRight,
      className: sensitiveTone(isSensitiveMasked, report.changes.daily_total_asset_change),
    },
    {
      label: "总仓位",
      value: formatPercent(latest.position_percent),
      detail: `综合 ${latest.account_count} 个账户，市值 ${maskSensitive(isSensitiveMasked, formatMoney(latest.market_value))}`,
      icon: BarChart3,
    },
  ];

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {cards.map((card) => (
        <Card key={card.label}>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">{card.label}</CardTitle>
            <card.icon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className={["tabular-nums text-2xl font-semibold", card.className || ""].join(" ")}>{card.value}</div>
            <p className="mt-1 text-xs text-muted-foreground">{card.detail}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
