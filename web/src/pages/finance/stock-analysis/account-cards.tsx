import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  formatMoney,
  formatPercent,
  formatSignedMoney,
  maskAccountIdentity,
  maskSensitive,
  sensitiveTone,
} from "./format";
import type { AccountSummary } from "./types";

export function AccountGrid({ accounts, isSensitiveMasked }: { accounts: AccountSummary[]; isSensitiveMasked: boolean }) {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      {accounts.map((account) => (
        <Card key={account.account_suffix}>
          <CardHeader className="flex flex-row items-start justify-between space-y-0">
            <div>
              <CardTitle>{maskAccountIdentity(isSensitiveMasked, account.account_alias)}</CardTitle>
              <CardDescription>
                {account.snapshot_date} / {isSensitiveMasked ? "账户信息已隐藏" : account.broker_name}
              </CardDescription>
            </div>
            <Badge variant="outline">{isSensitiveMasked ? "**••••" : `**${account.account_suffix}`}</Badge>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <AccountMetric label="总资产" value={maskSensitive(isSensitiveMasked, formatMoney(account.total_asset))} />
              <AccountMetric label="总市值" value={maskSensitive(isSensitiveMasked, formatMoney(account.market_value))} />
              <AccountMetric label="可用资金" value={maskSensitive(isSensitiveMasked, formatMoney(account.available_cash))} />
              <AccountMetric label="仓位" value={formatPercent(account.position_percent)} />
            </div>
            <div className="mt-3 flex flex-wrap gap-2 text-xs">
              <Badge variant="secondary" className={sensitiveTone(isSensitiveMasked, account.daily_change)}>
                本次 {maskSensitive(isSensitiveMasked, formatSignedMoney(account.daily_change))}
              </Badge>
              <Badge variant="secondary" className={sensitiveTone(isSensitiveMasked, account.cumulative_change)}>
                累计 {maskSensitive(isSensitiveMasked, formatSignedMoney(account.cumulative_change))}
              </Badge>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

export function AccountMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-muted/60 px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 break-words text-sm font-medium tabular-nums">{value}</div>
    </div>
  );
}
