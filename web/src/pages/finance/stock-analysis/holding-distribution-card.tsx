import { PieChart } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatMoney, formatPercent, maskSensitive, toNumber } from "./format";
import type { HoldingDistribution, TimelinePoint } from "./types";

export function HoldingDistributionCard({
  holdings,
  latest,
  isSensitiveMasked,
}: {
  holdings: HoldingDistribution[];
  latest: TimelinePoint;
  isSensitiveMasked: boolean;
}) {
  const total = holdings.reduce((sum, item) => sum + toNumber(item.market_value), 0);
  const maxValue = Math.max(...holdings.map((item) => toNumber(item.market_value)), 0);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>持仓分布</CardTitle>
            <CardDescription>{latest.snapshot_date} / 综合两个账户的标的市值和现金。</CardDescription>
          </div>
          <PieChart className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
      </CardHeader>
      <CardContent>
        {!holdings.length ? (
          <div className="rounded-md border px-3 py-6 text-sm text-muted-foreground">
            当前历史记录没有标的明细。之后重新上传或新增截图时，会自动记录每个标的的市值和现金并在这里展示分布。
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>资产分布合计</span>
              <span className="tabular-nums text-foreground">{maskSensitive(isSensitiveMasked, formatMoney(total))}</span>
            </div>
            {holdings.map((item) => {
              const width = maxValue > 0 ? Math.max((toNumber(item.market_value) / maxValue) * 100, 2) : 0;
              return (
                <div key={item.security_name} className="space-y-1.5">
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <div className="min-w-0">
                      <div className="truncate font-medium">{item.security_name}</div>
                      <div className="text-xs text-muted-foreground">
                        {isSensitiveMasked ? `${item.account_count} 个账户` : item.accounts || `${item.account_count} 个账户`}
                      </div>
                    </div>
                    <div className="shrink-0 text-right tabular-nums">
                      <div>{maskSensitive(isSensitiveMasked, formatMoney(item.market_value))}</div>
                      <div className="text-xs text-muted-foreground">{formatPercent(item.weight_percent)}</div>
                    </div>
                  </div>
                  <div className="h-2 rounded-full bg-muted">
                    <div className="h-2 rounded-full bg-foreground" style={{ width: `${width}%` }} />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
