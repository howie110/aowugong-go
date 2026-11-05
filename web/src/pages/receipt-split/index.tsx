import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { getReceiptStatistics, receiptRange, type ReceiptReport } from "@/lib/receipt-split";

import { ReceiptTrend } from "./trend";

/** ReceiptSplitPage 展示最近 30 天的每日使用用户趋势。 */
export function ReceiptSplitPage() {
  const [range, setRange] = useState(() => receiptRange(30));
  const [report, setReport] = useState<ReceiptReport | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(true);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    // 1. 固定使用最近 30 天，并忽略已经过期的请求。
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setReport(null);
    getReceiptStatistics(range.from, range.to, true, controller.signal)
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
    setRange(receiptRange(30));
    setRevision((value) => value + 1);
  }

  return (
    <div className="space-y-4" aria-busy={busy}>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <CardTitle>每日使用用户</CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">最近 30 天 · 北京时间</p>
            </div>
            <Button
              type="button"
              size="icon"
              variant="outline"
              aria-label="刷新小票统计"
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
              正在读取小票分账统计…
            </div>
          ) : error ? (
            <div
              role="alert"
              className="rounded-md border border-destructive/30 p-4 text-sm text-destructive"
            >
              {error}
              <Button variant="ghost" onClick={refresh}>
                重试
              </Button>
            </div>
          ) : report ? (
            <>
              <ReceiptTrend days={report.days} />
              {report.days.every(
                (day) => day.analysis_users === 0 && day.bill_users === 0,
              ) && (
                <p className="mt-4 text-center text-sm text-muted-foreground">
                  最近 30 天暂无使用记录
                </p>
              )}
            </>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
