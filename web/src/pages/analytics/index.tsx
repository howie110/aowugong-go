import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { beijingToday, shiftBeijingDate } from "@/lib/beijing-time";
import { getAnalytics, type AnalyticsReport } from "@/lib/analytics";

import { Trend } from "./charts";

/** last30Days 返回北京时间最近 30 个自然日的查询区间，结束时间不包含。 */
function last30Days() {
  const today = beijingToday();
  return {
    from: new Date(`${shiftBeijingDate(today, -29)}T00:00:00+08:00`),
    to: new Date(`${shiftBeijingDate(today, 1)}T00:00:00+08:00`),
  };
}

/** AnalyticsPage 展示最近 30 天的按天访问趋势。 */
export function AnalyticsPage() {
  const [range, setRange] = useState(last30Days);
  const [report, setReport] = useState<AnalyticsReport | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(true);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    // 1. 只读取总体趋势，忽略已经过期的请求。
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setReport(null);
    getAnalytics(range.from, range.to, {}, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setReport(result);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) {
          setError(cause instanceof Error ? cause.message : "读取失败");
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [range, revision]);

  /** refresh 重新取最近 30 天数据，跨天后也能回到当前窗口。 */
  function refresh() {
    setRange(last30Days());
    setRevision((value) => value + 1);
  }

  return (
    <div className="space-y-4" aria-busy={busy}>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <CardTitle>访问趋势</CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">最近 30 天 · 按天 · 北京时间</p>
            </div>
            <Button
              type="button"
              size="icon"
              variant="outline"
              aria-label="刷新访问统计"
              disabled={busy}
              onClick={refresh}
            >
              <RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {busy ? (
            <div role="status" className="py-16 text-center text-muted-foreground">
              正在读取访问统计…
            </div>
          ) : error ? (
            <div
              role="alert"
              className="rounded-md border border-destructive/40 p-4 text-sm text-destructive"
            >
              {error}
              <Button variant="ghost" onClick={refresh}>
                重试
              </Button>
            </div>
          ) : report ? (
            <>
              <Trend data={report.trend} />
              {report.current.views === 0 && (
                <p className="mt-4 text-center text-sm text-muted-foreground">
                  最近 30 天暂无访问记录
                </p>
              )}
            </>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
