import { Activity, CalendarClock, Database, FileText, LineChart, PlayCircle, Server, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { FinanceItem, FinanceMetric, FinancePageData } from "@/lib/finance";

const metricIconMap = [Server, CalendarClock, ShieldCheck, Activity];

export function OverviewContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <>
      <MetricGrid metrics={pageData.metrics || []} />
      <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
        <ItemListCard title="项目分布" description="当前页面和外部联动分布。" items={pageData.modules || []} icon="module" />
        <ItemListCard title="数据进度" description="核心数据表最新日期。" items={pageData.data_progress || []} icon="data" />
      </div>
    </>
  );
}

export function BacktestContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
      <ItemListCard title="回测结构" description="回测模块拆分和职责。" items={pageData.modules || []} icon="backtest" />
      <ItemListCard title="回测入口" description="后续页面表单会调用这些统一入口。" items={pageData.items || []} icon="file" />
    </div>
  );
}

export function DataContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <>
      <TableCard title="数据表" description="最新日期来自数据库 max 日期列。" items={pageData.tables || []} />
      <ItemListCard title="数据来源" description="数据来源和本地缓存策略。" items={pageData.sources || []} icon="data" />
    </>
  );
}

export function JobsContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <InfoCard title="任务入口" value={pageData.runner || "未知"} description="Registry 统一接收自动、页面和 CLI 执行。" />
        <InfoCard title="失败通知" value={pageData.fail_notify || "未知"} description="通知通过企业微信群机器人以纯文本发送。" />
      </div>
      <TableCard title="进程内任务" description="频率来自 Go 任务注册表，统一使用 Asia/Shanghai。" items={pageData.jobs || []} />
    </>
  );
}

export function TradingContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
      <ItemListCard title="交易保护" description="真实交易必须受总开关控制。" items={pageData.guards || []} icon="trade" />
      <ItemListCard title="交易模块" description="实盘交易和回测隔离。" items={pageData.modules || []} icon="module" />
    </div>
  );
}

export function NotificationsContent({ pageData }: { pageData: FinancePageData }) {
  return (
    <>
      <InfoCard title="微信通知" value={pageData.receiver_count ? "已启用" : "未启用"} description="这里只显示状态，不在页面暴露 token。" />
      <ItemListCard title="通知渠道" description="失败提醒和后续业务消息推送。" items={pageData.channels || []} icon="file" />
    </>
  );
}

export function LoadingCard() {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28" />)}
      </div>
      <Skeleton className="h-64" />
    </div>
  );
}

export function ErrorCard({ message }: { message: string }) {
  return (
    <Alert variant="destructive">
      <AlertTitle>页面数据加载失败</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

function MetricGrid({ metrics }: { metrics: FinanceMetric[] }) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {metrics.map((item, index) => {
        const Icon = metricIconMap[index] || Activity;
        return (
          <Card key={item.label}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground">{item.label}</CardTitle>
              <Icon className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="tabular-nums text-2xl font-semibold">{item.value}</div>
              <p className="mt-1 text-xs text-muted-foreground">{item.detail}</p>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

function ItemListCard({ title, description, items, icon }: { title: string; description: string; items: FinanceItem[]; icon: string }) {
  const Icon = icon === "data" ? Database : icon === "backtest" ? LineChart : icon === "trade" ? PlayCircle : icon === "file" ? FileText : LineChart;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {items.map((item) => (
          <div key={item.name} className="flex items-center justify-between gap-4 rounded-md border px-3 py-2">
            <div className="min-w-0">
              <div className="truncate font-mono text-sm">{item.name}</div>
              <div className="mt-1 text-xs text-muted-foreground">{item.description || item.entry || item.value || item.latest}</div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {item.status ? <StatusBadge status={item.status} /> : null}
              <Icon className="h-4 w-4 text-muted-foreground" />
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function TableCard({ title, description, items }: { title: string; description: string; items: FinanceItem[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead>说明</TableHead>
              <TableHead>日期/频率</TableHead>
              <TableHead className="text-right">状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => (
              <TableRow key={item.name}>
                <TableCell className="font-mono text-xs">{item.name}</TableCell>
                <TableCell className="text-muted-foreground">{item.description || item.command || "-"}</TableCell>
                <TableCell className="text-muted-foreground">{item.latest || item.schedule || item.date_column || "-"}</TableCell>
                <TableCell className="text-right">{item.status ? <StatusBadge status={item.status} /> : <Badge variant="outline">正常</Badge>}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function InfoCard({ title, value, description }: { title: string; value: string; description: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="break-all font-mono text-sm">{value}</div>
      </CardContent>
    </Card>
  );
}

function StatusBadge({ status }: { status: string }) {
  if (status === "active" || status === "ready" || status === "存在" || status === "safe" || status === "normal") {
    return <Badge variant="success">{status}</Badge>;
  }
  if (status === "danger") {
    return <Badge variant="danger">{status}</Badge>;
  }
  return <Badge variant="secondary">{status}</Badge>;
}
