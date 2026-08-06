import { ExternalLink, RefreshCw, Radar } from "lucide-react";
import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  ServiceMonitorResult,
  ServiceMonitorSummary,
  checkMonitoringServices,
  fetchMonitoringSummary,
} from "@/lib/monitoring";
import { notify } from "@/lib/notify";

export function MonitoringPage() {
  const [summary, setSummary] = useState<ServiceMonitorSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isChecking, setIsChecking] = useState(false);

  async function loadData() {
    setIsLoading(true);
    try {
      setSummary(await fetchMonitoringSummary());
    } catch (error) {
      notify.errorFrom(error, "监控数据加载失败", "加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleCheckNow() {
    setIsChecking(true);
    try {
      const result = await checkMonitoringServices();
      const description = `正常 ${result.up_count} 个，异常 ${result.down_count} 个。`;
      if (result.down_count > 0) {
        notify.warning("检测完成，有异常服务", description);
      } else {
        notify.success("检测完成", description);
      }
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "服务检测失败");
    } finally {
      setIsChecking(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  if (isLoading && !summary) {
    return <MonitoringSkeleton />;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={() => void loadData()} disabled={isLoading || isChecking}>
          {isLoading ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
          刷新
        </Button>
        <Button type="button" size="sm" onClick={() => void handleCheckNow()} disabled={isChecking}>
          {isChecking ? <Spinner /> : <Radar className="h-4 w-4" />}
          立即检测
        </Button>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {(summary?.metrics || []).map((item) => (
          <Card key={item.label}>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm text-muted-foreground">{item.label}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-semibold tabular-nums">{item.value}</div>
              <p className="mt-1 text-xs text-muted-foreground">{item.detail}</p>
            </CardContent>
          </Card>
        ))}
      </div>

      <ServiceMonitorCard services={summary?.services || []} />
    </div>
  );
}

function ServiceMonitorCard({ services }: { services: ServiceMonitorResult[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>服务监控</CardTitle>
        <CardDescription>每天 22:00 自动检测一次，HTTP 5xx 或无法连接记为异常。</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[18%]">服务</TableHead>
              <TableHead className="w-[34%]">地址</TableHead>
              <TableHead className="w-[10%]">状态</TableHead>
              <TableHead className="w-[10%]">HTTP</TableHead>
              <TableHead className="w-[10%]">耗时</TableHead>
              <TableHead className="w-[18%]">上次检测</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {services.map((service) => (
              <TableRow key={service.target_code}>
                <TableCell>
                  <div className="font-medium">{service.target_name}</div>
                  <div className="text-xs text-muted-foreground">{service.error_message || service.target_code}</div>
                </TableCell>
                <TableCell className="max-w-[24rem] truncate font-mono text-xs text-muted-foreground">
                  <MonitorTargetLink url={service.target_url} />
                </TableCell>
                <TableCell>
                  <MonitorStatusBadge status={service.status} />
                </TableCell>
                <TableCell className="tabular-nums text-muted-foreground">{service.http_status ?? "-"}</TableCell>
                <TableCell className="tabular-nums text-muted-foreground">{service.latency_ms == null ? "-" : `${service.latency_ms}ms`}</TableCell>
                <TableCell className="text-muted-foreground">{formatDate(service.checked_at)}</TableCell>
              </TableRow>
            ))}
            {!services.length ? (
              <TableRow>
                <TableCell colSpan={6}>
                  <Empty className="border-0">
                    <EmptyHeader>
                      <EmptyTitle>暂无监控服务</EmptyTitle>
                      <EmptyDescription>监控目标配置完成后会显示检测结果。</EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

/** MonitoringSkeleton 展示服务监控页面加载占位，无副作用。 */
function MonitoringSkeleton() {
  // 1. 保留指标和监控表尺寸，避免加载完成时布局跳动。
  return (
    <div className="space-y-4">
      <Skeleton className="ml-auto h-8 w-40" />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28" />)}
      </div>
      <Skeleton className="h-72" />
    </div>
  );
}

function MonitorTargetLink({ url }: { url: string }) {
  if (!isHttpUrl(url)) {
    return <span>{url}</span>;
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      title={url}
      className="inline-flex max-w-full items-center gap-1 truncate text-foreground underline-offset-4 hover:underline"
    >
      <span className="truncate">{url}</span>
      <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
    </a>
  );
}

function isHttpUrl(url: string) {
  return /^https?:\/\//i.test(url);
}

function MonitorStatusBadge({ status }: { status: string }) {
  if (status === "up") {
    return <Badge variant="success">正常</Badge>;
  }
  if (status === "down") {
    return <Badge variant="danger">异常</Badge>;
  }
  return <Badge variant="secondary">未检测</Badge>;
}

function formatDate(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
