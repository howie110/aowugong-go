import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { AccountMetric } from "./account-cards";
import { formatMoney, formatPercent, formatSignedMoney, maskSensitive, sensitiveTone } from "./format";
import type { TimelinePoint } from "./types";

export function RecentTimelineTable({ data, isSensitiveMasked }: { data: TimelinePoint[]; isSensitiveMasked: boolean }) {
  const recent = [...data].reverse().slice(0, 10);
  return (
    <Card>
      <CardHeader>
        <CardTitle>最近记录</CardTitle>
        <CardDescription>按上传日期倒序展示组合层面的聚合结果，适合每周记录。</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-3 md:hidden">
          {recent.map((item) => (
            <div key={item.snapshot_date} className="rounded-md border px-3 py-3">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-medium">{item.snapshot_date}</div>
                  <div className="mt-1 text-xs text-muted-foreground">{item.account_count} 个账户</div>
                </div>
                <Badge variant="outline">{formatPercent(item.position_percent)}</Badge>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-2">
                <AccountMetric label="总资产" value={maskSensitive(isSensitiveMasked, formatMoney(item.total_asset))} />
                <AccountMetric label="本次变化" value={maskSensitive(isSensitiveMasked, formatSignedMoney(item.daily_change))} />
                <AccountMetric label="总市值" value={maskSensitive(isSensitiveMasked, formatMoney(item.market_value))} />
                <AccountMetric label="可用资金" value={maskSensitive(isSensitiveMasked, formatMoney(item.available_cash))} />
              </div>
            </div>
          ))}
        </div>
        <div className="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>日期</TableHead>
                <TableHead className="text-right">总资产</TableHead>
                <TableHead className="text-right">总市值</TableHead>
                <TableHead className="text-right">可用资金</TableHead>
                <TableHead className="text-right">本次变化</TableHead>
                <TableHead className="text-right">总仓位</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {recent.map((item) => (
                <TableRow key={item.snapshot_date}>
                  <TableCell className="whitespace-nowrap">{item.snapshot_date}</TableCell>
                  <TableCell className="text-right tabular-nums">{maskSensitive(isSensitiveMasked, formatMoney(item.total_asset))}</TableCell>
                  <TableCell className="text-right tabular-nums">{maskSensitive(isSensitiveMasked, formatMoney(item.market_value))}</TableCell>
                  <TableCell className="text-right tabular-nums">{maskSensitive(isSensitiveMasked, formatMoney(item.available_cash))}</TableCell>
                  <TableCell className={["text-right tabular-nums", sensitiveTone(isSensitiveMasked, item.daily_change)].join(" ")}>
                    {maskSensitive(isSensitiveMasked, formatSignedMoney(item.daily_change))}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{formatPercent(item.position_percent)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}
