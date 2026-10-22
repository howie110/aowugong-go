import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  getReceiptStatistics,
  receiptRange,
  shiftReceiptDate,
  type ReceiptReport,
} from "@/lib/receipt-split";
import { ReceiptTrend } from "./trend";
const count = (n: number) => n.toLocaleString("zh-CN");
export function ReceiptSplitPage() {
  const [range, setRange] = useState(() => receiptRange(30));
  const [from, setFrom] = useState(range.from);
  const [through, setThrough] = useState(shiftReceiptDate(range.to, -1));
  const [exclude, setExclude] = useState(true);
  const [report, setReport] = useState<ReceiptReport | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(true);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setReport(null);
    getReceiptStatistics(range.from, range.to, exclude, controller.signal)
      .then((r) => {
        if (!controller.signal.aborted) setReport(r);
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "读取失败");
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [range, exclude, revision]);
  function preset(days: number) {
    const next = receiptRange(days);
    setRange(next);
    setFrom(next.from);
    setThrough(shiftReceiptDate(next.to, -1));
  }
  const metrics = report
    ? [
        {
          label: "历史登录用户",
          value: count(report.lifetime_login_users),
          hint: "全部已有登录记录去重，不受日期筛选影响",
        },
        {
          label: "识别用户",
          value: count(report.totals.analysis_users),
          hint: "所选周期提交过已受理识别任务的用户",
        },
        {
          label: "识别任务",
          value: count(report.totals.tasks),
          hint: "包含成功、失败及处理中任务",
        },
        {
          label: "生成账单用户",
          value: count(report.totals.bill_users),
          hint: "所选周期成功保存账单的用户",
        },
        {
          label: "生成账单",
          value: count(report.totals.bills),
          hint: "包含后来隐藏或删除的历史账单",
        },
        {
          label: "识别成功率",
          value:
            report.totals.succeeded + report.totals.failed
              ? `${((100 * report.totals.succeeded) / (report.totals.succeeded + report.totals.failed)).toFixed(1)}%`
              : "—",
          hint: "成功 ÷ 已完成任务，不含处理中任务",
        },
      ]
    : [];
  return (
    <div className="space-y-5" aria-busy={busy}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          小票 A 账 · 用户与实际使用 · 北京时间
        </p>
        <div className="flex gap-2">
          {[1, 7, 30, 90].map((n) => (
            <Button
              key={n}
              size="sm"
              variant="outline"
              onClick={() => preset(n)}
            >
              {n === 1 ? "今天" : `近 ${n} 天`}
            </Button>
          ))}
          <Button
            size="icon"
            variant="outline"
            aria-label="刷新小票统计"
            disabled={busy}
            onClick={() => setRevision((n) => n + 1)}
          >
            <RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </div>
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!from || !through || through < from) {
            setError("请选择有效日期范围");
            return;
          }
          const to = shiftReceiptDate(through, 1);
          if ((Date.parse(to) - Date.parse(from)) / 86400000 > 366) {
            setError("最多查询 366 天");
            return;
          }
          setRange({ from, to });
        }}
      >
        <label className="text-xs text-muted-foreground">
          开始日期
          <Input
            type="date"
            aria-label="开始日期"
            required
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </label>
        <label className="text-xs text-muted-foreground">
          结束日期（含）
          <Input
            type="date"
            aria-label="结束日期"
            required
            value={through}
            onChange={(e) => setThrough(e.target.value)}
          />
        </label>
        <Button type="submit" variant="outline">
          应用
        </Button>
        <label className="flex h-9 items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={exclude}
            onChange={(e) => setExclude(e.target.checked)}
          />
          排除运营者
        </label>
      </form>
      <p className="text-xs text-muted-foreground">
        当前范围：{range.from} — {shiftReceiptDate(range.to, -1)} ·
        固定开发账号始终排除
      </p>
      {busy && (
        <div role="status" className="py-16 text-center text-muted-foreground">
          正在读取小票分账统计…
        </div>
      )}
      {error && (
        <div
          role="alert"
          className="rounded-md border border-destructive/30 p-4 text-sm text-destructive"
        >
          {error}
          <Button variant="ghost" onClick={() => setRevision((n) => n + 1)}>
            重试
          </Button>
        </div>
      )}
      {report && !busy && (
        <>
          {!report.operators_configured && exclude && (
            <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
              尚未配置运营者名单，目前只排除固定开发账号。
            </p>
          )}
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {metrics.map((m) => (
              <Card key={m.label}>
                <CardContent className="pt-5">
                  <p className="text-sm text-muted-foreground">{m.label}</p>
                  <p className="my-3 text-3xl font-semibold tabular-nums">
                    {m.value}
                  </p>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {m.hint}
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-base">每日使用用户</CardTitle>
            </CardHeader>
            <CardContent>
              <ReceiptTrend days={report.days} />
              {report.totals.tasks === 0 && report.totals.bills === 0 && (
                <p className="mt-4 text-center text-sm text-muted-foreground">
                  所选范围暂无识别或账单记录
                </p>
              )}
            </CardContent>
          </Card>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            {[
              ["成功", report.totals.succeeded],
              ["失败", report.totals.failed],
              ["处理中", report.totals.running],
              ["已受理待处理", report.totals.queued],
            ].map(([label, n]) => (
              <div key={label} className="rounded-lg border p-4">
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="mt-2 text-xl font-semibold">{count(Number(n))}</p>
              </div>
            ))}
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-base">每日明细</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="max-h-[32rem] overflow-auto">
                <table className="w-full whitespace-nowrap text-sm">
                  <thead className="sticky top-0 bg-background">
                    <tr>
                      {[
                        "日期",
                        "识别用户",
                        "识别任务",
                        "成功",
                        "失败",
                        "处理中",
                        "待处理",
                        "账单用户",
                        "账单数",
                      ].map((x) => (
                        <th
                          key={x}
                          className="border-b px-3 py-3 text-right first:text-left font-medium"
                        >
                          {x}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {[...report.days].reverse().map((d) => (
                      <tr key={d.date}>
                        <td className="border-b px-3 py-2">{d.date}</td>
                        {[
                          d.analysis_users,
                          d.tasks,
                          d.succeeded,
                          d.failed,
                          d.running,
                          d.queued,
                          d.bill_users,
                          d.bills,
                        ].map((n, i) => (
                          <td
                            key={i}
                            className="border-b px-3 py-2 text-right tabular-nums"
                          >
                            {count(n)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
          <p className="text-xs leading-6 text-muted-foreground">
            用户按微信登录后的稳定标识去重；每日用户数相加不等于整个周期的去重用户数。任务按受理日期归属、状态为查询时状态，满员拒绝不计任务。分享页匿名浏览、仅打开小程序的人数及新增用户暂不统计。
          </p>
          <p className="text-xs text-muted-foreground">
            数据生成：
            {new Date(report.generated_at).toLocaleString("zh-CN", {
              timeZone: "Asia/Shanghai",
              hour12: false,
            })}{" "}
            · 汇总最多缓存 30 秒
          </p>
        </>
      )}
    </div>
  );
}
