import { useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, RefreshCw, X } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  getAnalytics,
  type AnalyticsReport,
  type Metrics,
} from "@/lib/analytics";
import { ANALYTICS_EXCLUSION_KEY } from "@/lib/analytics-tracker";
import { Trend, Heatmap, WorldMap, number, timeLabel } from "./charts";
import { RankCard, displayName, labels } from "./rank-card";

const selectClass = "h-9 rounded-md border bg-background px-3 text-sm";
const localInput = (date: Date) =>
  new Date(date.getTime() + 8 * 3600000).toISOString().slice(0, 16);
const metricLabels: Record<keyof Metrics, string> = {
  visitors: "访客",
  visits: "访问次数",
  views: "浏览量",
  bounce_rate: "跳出率",
  duration: "平均访问时长",
};
function format(key: keyof Metrics, n: number) {
  return key === "bounce_rate"
    ? `${number(n)}%`
    : key === "duration"
      ? `${number(n)}s`
      : number(n);
}
function comparison(key: keyof Metrics, current: number, previous: number) {
  if (key === "bounce_rate")
    return `${current - previous >= 0 ? "+" : ""}${number(current - previous)} 个百分点`;
  if (previous === 0) return current === 0 ? "无变化" : "前期为 0";
  const delta = ((current - previous) / previous) * 100;
  return `${delta >= 0 ? "+" : ""}${number(delta)}%`;
}
export function AnalyticsPage() {
  const [range, setRange] = useState(() => {
    const to = new Date();
    return { from: new Date(to.getTime() - 86400000), to };
  });
  const [preset, setPreset] = useState("24");
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [report, setReport] = useState<AnalyticsReport | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(true);
  const [revision, setRevision] = useState(0);
  const [dimension, setDimension] = useState("path");
  const [value, setValue] = useState("");
  const [customFrom, setCustomFrom] = useState(localInput(range.from));
  const [customTo, setCustomTo] = useState(localInput(range.to));
  const [excluded, setExcluded] = useState(() => {
    try {
      return localStorage.getItem(ANALYTICS_EXCLUSION_KEY) === "1";
    } catch {
      return false;
    }
  });
  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setReport(null);
    getAnalytics(range.from, range.to, filters, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setReport(result);
      })
      .catch((err) => {
        if (!controller.signal.aborted)
          setError(err instanceof Error ? err.message : "读取失败");
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [range, filters, revision]);
  function filter(key: string, v: string) {
    setFilters((old) => ({ ...old, [key]: v }));
  }
  function selectPeriod(hours: string) {
    setPreset(hours);
    if (hours !== "custom") {
      const to = new Date();
      setRange({ from: new Date(to.getTime() - Number(hours) * 3600000), to });
    }
  }
  function shift(sign: number) {
    const duration = range.to.getTime() - range.from.getTime();
    setRange({
      from: new Date(range.from.getTime() + sign * duration),
      to: new Date(range.to.getTime() + sign * duration),
    });
  }
  function toggleExclusion() {
    try {
      localStorage.setItem(ANALYTICS_EXCLUSION_KEY, excluded ? "0" : "1");
      setExcluded(!excluded);
    } catch {
      setError("浏览器禁止保存设置，无法更改采集排除状态");
    }
  }
  return (
    <div className="space-y-5" aria-busy={busy}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          全站页面 · 北京时间 · 匿名统计
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" size="sm" onClick={toggleExclusion}>
            {excluded ? "恢复本浏览器采集" : "排除本浏览器"}
          </Button>
          <Button
            variant="outline"
            size="icon"
            aria-label="上一周期"
            onClick={() => shift(-1)}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            aria-label="下一周期"
            disabled={range.to.getTime() >= Date.now() - 60000}
            onClick={() => shift(1)}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
          <select
            aria-label="时间范围"
            className={selectClass}
            value={preset}
            onChange={(e) => selectPeriod(e.target.value)}
          >
            <option value="24">最近 24 小时</option>
            <option value="168">最近 7 天</option>
            <option value="720">最近 30 天</option>
            <option value="2160">最近 90 天</option>
            <option value="custom">自定义</option>
          </select>
          <Button
            variant="outline"
            size="icon"
            aria-label="刷新统计"
            disabled={busy}
            onClick={() => {
              if (preset !== "custom") selectPeriod(preset);
              else setRevision((r) => r + 1);
            }}
          >
            <RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </div>
      {preset === "custom" && (
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const from = new Date(`${customFrom}:00+08:00`),
              to = new Date(`${customTo}:00+08:00`);
            if (
              !Number.isFinite(from.getTime()) ||
              !Number.isFinite(to.getTime()) ||
              to <= from ||
              to.getTime() - from.getTime() > 366 * 86400000
            ) {
              setError("请选择有效时间范围，最多 366 天");
              return;
            }
            setRange({ from, to });
          }}
        >
          <label className="text-xs">
            开始（北京时间）
            <Input
              type="datetime-local"
              value={customFrom}
              onChange={(e) => setCustomFrom(e.target.value)}
              required
            />
          </label>
          <label className="text-xs">
            结束（不包含）
            <Input
              type="datetime-local"
              value={customTo}
              onChange={(e) => setCustomTo(e.target.value)}
              required
            />
          </label>
          <Button type="submit" variant="outline">
            应用
          </Button>
        </form>
      )}
      <div className="text-xs text-muted-foreground">
        {timeLabel(range.from.toISOString())} —{" "}
        {timeLabel(range.to.toISOString())} · 与前一等长周期比较
      </div>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          filter(dimension, value);
          setValue("");
        }}
      >
        <select
          aria-label="筛选维度"
          className={selectClass}
          value={dimension}
          onChange={(e) => setDimension(e.target.value)}
        >
          {Object.entries(labels).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </select>
        <Input
          className="w-56"
          aria-label="精确匹配值"
          placeholder="精确匹配；留空匹配未知"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          maxLength={1000}
        />
        <Button variant="outline" type="submit">
          添加筛选
        </Button>
        {Object.keys(filters).length > 0 && (
          <Button type="button" variant="ghost" onClick={() => setFilters({})}>
            清除全部
          </Button>
        )}
      </form>
      {Object.keys(filters).length > 0 && (
        <div className="flex flex-wrap gap-2">
          {Object.entries(filters).map(([k, v]) => (
            <Button
              variant="secondary"
              size="sm"
              key={k}
              onClick={() =>
                setFilters((old) => {
                  const next = { ...old };
                  delete next[k];
                  return next;
                })
              }
            >
              {labels[k]}：
              <span className="max-w-48 truncate">{displayName(k, v)}</span>
              <X className="ml-1 h-3 w-3" />
            </Button>
          ))}
        </div>
      )}
      {error && (
        <div
          role="alert"
          className="rounded-md border border-destructive/40 p-4 text-sm text-destructive"
        >
          {error}
          <Button variant="ghost" onClick={() => setRevision((r) => r + 1)}>
            重试
          </Button>
        </div>
      )}
      {busy && (
        <div
          role="status"
          className="py-16 text-center text-sm text-muted-foreground"
        >
          正在读取访问统计…
        </div>
      )}
      {report && !busy && (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
            {(Object.keys(metricLabels) as (keyof Metrics)[]).map((key) => (
              <Card key={key}>
                <CardContent className="pt-5">
                  <div className="text-sm text-muted-foreground">
                    {metricLabels[key]}
                  </div>
                  <div className="my-3 text-3xl font-semibold tabular-nums">
                    {format(key, report.current[key])}
                  </div>
                  <div
                    className="text-xs text-muted-foreground"
                    title={`前期：${format(key, report.previous[key])}`}
                  >
                    {comparison(key, report.current[key], report.previous[key])}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
          <Card>
            <CardHeader className="pb-0">
              <CardTitle className="text-base">访问趋势</CardTitle>
            </CardHeader>
            <CardContent>
              <Trend data={report.trend} />
              {report.current.views === 0 && (
                <p className="text-center text-xs text-muted-foreground">
                  所选范围暂无浏览记录
                </p>
              )}
            </CardContent>
          </Card>
          <div className="grid gap-4 xl:grid-cols-2">
            <RankCard
              title="网页"
              tabs={["path", "url", "entry", "exit"]}
              ranks={report.ranks}
              onFilter={filter}
            />
            <RankCard
              title="来源"
              tabs={["referrer", "channel"]}
              ranks={report.ranks}
              onFilter={filter}
            />
            <RankCard
              title="环境"
              tabs={["browser", "os", "device", "language"]}
              ranks={report.ranks}
              onFilter={filter}
            />
            <RankCard
              title="位置"
              tabs={["country", "region", "city"]}
              ranks={report.ranks}
              onFilter={filter}
            />
          </div>
          <div className="grid gap-4 xl:grid-cols-[2fr_1fr]">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">地理分布</CardTitle>
              </CardHeader>
              <CardContent>
                <WorldMap
                  data={report.ranks.country || []}
                  onCountry={(c) => filter("country", c)}
                />
                <p className="mt-3 text-xs text-muted-foreground">
                  {report.geo_enabled
                    ? "位置由 IP 地理库估计，可能受代理或 VPN 影响。"
                    : "尚未配置本地地理库，位置显示未知；其他统计不受影响。"}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  地图数据：Natural Earth（公共领域）
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="text-base">流量时段</CardTitle>
              </CardHeader>
              <CardContent>
                <Heatmap data={report.heat} />
                <p className="mt-3 text-xs text-muted-foreground">
                  按星期与小时累计浏览量，颜色越深访问越多。
                </p>
              </CardContent>
            </Card>
          </div>
          <p className="text-xs leading-6 text-muted-foreground">
            访客按匿名标识估计，跨月轮换；30
            分钟无浏览后开始新访问。跳出率和时长基于当前范围及筛选后的浏览记录，单页访问记为
            0 秒。入口与退出页按整次访问计算，进行中的退出页可能变化。
          </p>
        </>
      )}
    </div>
  );
}
