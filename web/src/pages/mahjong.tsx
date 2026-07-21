import { useEffect, useMemo, useState } from "react";
import {
  BarChart3,
  CalendarDays,
  Clock3,
  Percent,
  RefreshCw,
  Save,
  Target,
  TrendingDown,
  TrendingUp,
  Trophy,
} from "lucide-react";

import { DatePicker } from "@/components/date-picker";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Spinner } from "@/components/ui/spinner";
import { buildMahjongHeatmap, getMahjongHeatmapCellColor, type MahjongHeatmap, type MahjongHeatmapDay } from "@/lib/mahjong-heatmap";
import { type MahjongReport, type MahjongTimelinePoint, fetchMahjongReport, saveMahjongRecord } from "@/lib/mahjong";
import { notify } from "@/lib/notify";

const chartLeft = 58;
const chartRight = 710;
const chartTop = 18;
const chartBottom = 226;

export function MahjongPage() {
  const [report, setReport] = useState<MahjongReport | null>(null);
  const [playedDate, setPlayedDate] = useState(getTodayText());
  const [resultAmount, setResultAmount] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    loadReport();
  }, []);

  async function loadReport() {
    setIsLoading(true);
    try {
      setReport(await fetchMahjongReport());
    } catch (error) {
      notify.errorFrom(error, "读取麻将战绩失败");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleSave() {
    if (!playedDate) {
      notify.warning("请选择日期");
      return;
    }
    if (!resultAmount.trim() || Number.isNaN(Number(resultAmount))) {
      notify.warning("请输入当日输赢");
      return;
    }

    setIsSaving(true);
    try {
      const result = await saveMahjongRecord(playedDate, resultAmount);
      setResultAmount("");
      notify.success(result.status === "inserted" ? "战绩已新增" : result.status === "updated" ? "战绩已更新" : "战绩无变化");
      await loadReport();
    } catch (error) {
      notify.errorFrom(error, "保存麻将战绩失败");
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <RecordEditorCard
        playedDate={playedDate}
        resultAmount={resultAmount}
        isSaving={isSaving}
        isLoading={isLoading}
        onPlayedDateChange={setPlayedDate}
        onResultAmountChange={setResultAmount}
        onSave={handleSave}
        onRefresh={loadReport}
      />
      {report ? (
        <>
          <SummaryCards report={report} />
          <TrendCard report={report} />
          <FrequencyCard report={report} />
        </>
      ) : (
        <Empty>
          <EmptyHeader>
            <EmptyMedia>{isLoading ? <Spinner /> : <Trophy />}</EmptyMedia>
            <EmptyTitle>{isLoading ? "正在读取麻将战绩" : "暂无麻将战绩"}</EmptyTitle>
            <EmptyDescription>{isLoading ? "正在同步最新记录。" : "录入第一条战绩后会显示统计和趋势。"}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
    </div>
  );
}

function RecordEditorCard({
  playedDate,
  resultAmount,
  isSaving,
  isLoading,
  onPlayedDateChange,
  onResultAmountChange,
  onSave,
  onRefresh,
}: {
  playedDate: string;
  resultAmount: string;
  isSaving: boolean;
  isLoading: boolean;
  onPlayedDateChange: (value: string) => void;
  onResultAmountChange: (value: string) => void;
  onSave: () => void;
  onRefresh: () => void;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <CardTitle>麻将战绩</CardTitle>
          <CardDescription>录入某天的当日输赢；同一天重复保存会覆盖旧记录。</CardDescription>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onRefresh} disabled={isLoading}>
          {isLoading ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
          刷新
        </Button>
      </CardHeader>
      <CardContent>
        <div className="grid gap-3 md:grid-cols-[180px_1fr_auto] md:items-end">
          <div className="space-y-2">
            <Label htmlFor="mahjong-date">日期</Label>
            <DatePicker id="mahjong-date" value={playedDate} onChange={onPlayedDateChange} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="mahjong-result">当日输赢</Label>
            <Input
              id="mahjong-result"
              type="number"
              inputMode="decimal"
              step="0.5"
              placeholder="例如 -30 或 40.5"
              value={resultAmount}
              onChange={(event) => onResultAmountChange(event.target.value)}
            />
          </div>
          <Button type="button" disabled={isSaving} onClick={onSave} className="w-full md:w-auto">
            {isSaving ? <Spinner /> : <Save className="h-4 w-4" />}
            保存
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function SummaryCards({ report }: { report: MahjongReport }) {
  const summary = report.summary;
  const cards = [
    {
      label: "总输赢",
      value: formatSigned(summary.total_result),
      detail: `${summary.first_date || "-"} 至 ${summary.latest_date || "-"}`,
      icon: Trophy,
      tone: amountTone(summary.total_result),
    },
    {
      label: "胜率",
      value: `${formatNumber(summary.win_rate)}%`,
      detail: `${summary.win_games} 胜 / ${summary.loss_games} 负 / ${summary.draw_games} 平`,
      icon: Percent,
    },
    {
      label: "场均",
      value: formatSigned(summary.average_result),
      detail: `共 ${summary.total_games} 场，跨度 ${summary.span_days} 天`,
      icon: BarChart3,
      tone: amountTone(summary.average_result),
    },
    {
      label: "实际场均",
      value: formatSigned(summary.adjusted_average_result),
      detail: `按场费 ${formatNumber(summary.table_fee)} 修正`,
      icon: Target,
      tone: amountTone(summary.adjusted_average_result),
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
            <div className={["tabular-nums text-2xl font-semibold", card.tone || ""].join(" ")}>{card.value}</div>
            <p className="mt-1 text-xs text-muted-foreground">{card.detail}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function TrendCard({ report }: { report: MahjongReport }) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>累计输赢趋势</CardTitle>
            <CardDescription>移动到图上查看某一场的日期、当日输赢、累计输赢和滚动场均。</CardDescription>
          </div>
          <TrendingUp className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
      </CardHeader>
      <CardContent>
        <MahjongTrendSvg report={report} />
      </CardContent>
    </Card>
  );
}

function MahjongTrendSvg({ report }: { report: MahjongReport }) {
  const chart = useMemo(() => buildTrendChart(report.timeline), [report.timeline]);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const activeIndex = hoveredIndex ?? Math.max(report.timeline.length - 1, 0);
  const activePoint = report.timeline[activeIndex];
  const activeChartPoint = chart.points[activeIndex];
  const tooltipX = activeChartPoint ? Math.min(Math.max(activeChartPoint.x + 14, 90), 530) : 90;
  const tooltipY = activeChartPoint ? Math.min(Math.max(activeChartPoint.y - 58, 26), 150) : 30;

  function handlePointerMove(event: React.PointerEvent<SVGRectElement>) {
    if (!report.timeline.length) {
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    const xInViewBox = ((event.clientX - rect.left) / rect.width) * 760;
    const ratio = Math.min(Math.max((xInViewBox - chartLeft) / (chartRight - chartLeft), 0), 1);
    const index = Math.round(ratio * (report.timeline.length - 1));
    setHoveredIndex(index);
  }

  if (!report.timeline.length) {
    return (
      <Empty>
        <EmptyHeader><EmptyTitle>暂无趋势数据</EmptyTitle></EmptyHeader>
      </Empty>
    );
  }

  return (
    <div className="space-y-3">
      <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
        <LegendDot color="#111827" label="累计输赢" value={formatSigned(report.summary.total_result)} />
        <LegendDot color="#16a34a" label="盈利场次" value={`${report.summary.win_games} 场`} />
        <LegendDot color="#dc2626" label="亏损场次" value={`${report.summary.loss_games} 场`} />
      </div>
      <svg viewBox="0 0 760 280" className="h-72 w-full touch-none select-none" role="img" aria-label="麻将累计输赢趋势图">
        {chart.gridLines.map((line) => (
          <g key={line.y}>
            <line x1={chartLeft} x2={chartRight} y1={line.y} y2={line.y} stroke="#e5e7eb" strokeWidth="1" />
            <text x="50" y={line.y + 4} textAnchor="end" className="fill-muted-foreground text-[10px]">
              {formatCompact(line.value)}
            </text>
          </g>
        ))}
        <line x1={chartLeft} x2={chartRight} y1={chart.zeroY} y2={chart.zeroY} stroke="#a1a1aa" strokeDasharray="4 4" />
        {chart.bars.map((bar) => (
          <rect key={bar.key} x={bar.x} y={bar.y} width={bar.width} height={bar.height} rx="2" fill={bar.color} opacity="0.72" />
        ))}
        <path d={chart.linePath} fill="none" stroke="#111827" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2.5" />
        {chart.points.map((point, index) => (
          <circle key={point.key} cx={point.x} cy={point.y} r={index === activeIndex ? "5" : "2.8"} fill="#111827" />
        ))}
        {activeChartPoint && activePoint ? (
          <g>
            <line x1={activeChartPoint.x} x2={activeChartPoint.x} y1={chartTop} y2={chartBottom} stroke="#111827" strokeDasharray="4 4" opacity="0.55" />
            <line x1={chartLeft} x2={chartRight} y1={activeChartPoint.y} y2={activeChartPoint.y} stroke="#111827" strokeDasharray="4 4" opacity="0.25" />
            <rect x={tooltipX} y={tooltipY} width="218" height="92" rx="6" fill="#ffffff" stroke="#d4d4d8" />
            <text x={tooltipX + 12} y={tooltipY + 20} className="fill-foreground text-[12px] font-semibold">
              第 {activePoint.sequence} 场 / {activePoint.played_date}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 40} className={["text-[11px]", amountSvgTone(activePoint.result_amount)].join(" ")}>
              当日：{formatSigned(activePoint.result_amount)}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 58} className={["text-[11px]", amountSvgTone(activePoint.cumulative_result)].join(" ")}>
              累计：{formatSigned(activePoint.cumulative_result)}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 76} className={["text-[11px]", amountSvgTone(activePoint.running_average)].join(" ")}>
              滚动场均：{formatSigned(activePoint.running_average)}
            </text>
          </g>
        ) : null}
        <text x={chartLeft} y="254" className="fill-muted-foreground text-[10px]">
          {report.timeline[0].played_date}
        </text>
        <text x={chartRight} y="254" textAnchor="end" className="fill-muted-foreground text-[10px]">
          {report.timeline[report.timeline.length - 1].played_date}
        </text>
        <rect
          x={chartLeft}
          y={chartTop}
          width={chartRight - chartLeft}
          height={chartBottom - chartTop}
          fill="transparent"
          onPointerMove={handlePointerMove}
          onPointerLeave={() => setHoveredIndex(null)}
        />
      </svg>
      <div className="grid gap-2 text-sm md:grid-cols-2">
        <ExtremeDay label="最佳单日" value={report.summary.best_day.result_amount} date={report.summary.best_day.played_date} icon={TrendingUp} />
        <ExtremeDay label="最差单日" value={report.summary.worst_day.result_amount} date={report.summary.worst_day.played_date} icon={TrendingDown} />
      </div>
    </div>
  );
}

function FrequencyCard({ report }: { report: MahjongReport }) {
  const frequency = report.frequency;
  const heatmap = useMemo(() => buildMahjongHeatmap(report.timeline), [report.timeline]);
  const maxWeekdayCount = Math.max(...frequency.weekday_distribution.map((item) => item.game_count), 1);
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>打牌频率</CardTitle>
            <CardDescription>按最新记录往前统计近 90 天和近一年，同时观察间隔、月份和星期习惯。</CardDescription>
          </div>
          <CalendarDays className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <FrequencyMetric label="近 90 天" value={`${frequency.recent_90_day_games} 场`} detail="从最新记录向前 90 天" icon={CalendarDays} />
          <FrequencyMetric label="近一年" value={`${frequency.recent_365_day_games} 场`} detail="从最新记录向前 365 天" icon={CalendarDays} />
          <FrequencyMetric label="平均间隔" value={`${formatNumber(frequency.average_days_between_games)} 天`} detail="相邻两次记录的平均天数" icon={Clock3} />
          <FrequencyMetric label="月均频率" value={`${formatNumber(frequency.average_games_per_month)} 场`} detail={`${frequency.active_months} 个活跃月份`} icon={BarChart3} />
          <FrequencyMetric
            label="最密集月份"
            value={frequency.most_active_month || "-"}
            detail={`${frequency.most_active_month_games} 场`}
            icon={TrendingUp}
          />
          <FrequencyMetric
            label="最长空窗"
            value={`${frequency.longest_gap.days} 天`}
            detail={
              frequency.longest_gap.start_date && frequency.longest_gap.end_date
                ? `${frequency.longest_gap.start_date} 至 ${frequency.longest_gap.end_date}`
                : "-"
            }
            icon={Clock3}
          />
        </div>
        <FrequencyHeatmap heatmap={heatmap} />
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3 text-sm">
            <div className="font-medium">星期分布</div>
            <div className="text-xs text-muted-foreground">
              最常打：{frequency.favorite_weekday || "-"} / {frequency.favorite_weekday_games} 场
            </div>
          </div>
          <div className="grid gap-2 md:grid-cols-7">
            {frequency.weekday_distribution.map((item) => {
              const width = Math.max((item.game_count / maxWeekdayCount) * 100, item.game_count ? 8 : 2);
              return (
                <div key={item.weekday} className="rounded-md border px-2 py-2">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-medium">{item.label}</span>
                    <span className="tabular-nums text-muted-foreground">{item.game_count}</span>
                  </div>
                  <Progress value={width} className="mt-2" />
                  <div className="mt-1 text-right text-[10px] tabular-nums text-muted-foreground">{formatNumber(item.weight_percent)}%</div>
                </div>
              );
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function FrequencyHeatmap({ heatmap }: { heatmap: MahjongHeatmap }) {
  const latestActiveDay = [...heatmap.days].reverse().find((day) => day.count > 0) || null;
  const [activeDate, setActiveDate] = useState<string | null>(null);
  const activeDay = heatmap.days.find((day) => day.date === activeDate) || latestActiveDay;

  if (!heatmap.weeks.length) {
    return (
      <Empty>
        <EmptyHeader><EmptyTitle>暂无打牌频率数据</EmptyTitle></EmptyHeader>
      </Empty>
    );
  }

  return (
    <div className="space-y-3 rounded-md border px-3 py-3">
      <div className="flex flex-col gap-1 md:flex-row md:items-end md:justify-between">
        <div>
          <div className="text-sm font-medium">近一年热力图</div>
          <div className="text-xs text-muted-foreground">
            {heatmap.startDate} 至 {heatmap.endDate}，{heatmap.totalActiveDays} 天有记录 / {heatmap.totalGames} 场
          </div>
        </div>
        <HeatmapLegend />
      </div>
      <div className="flex gap-2">
        <div className="grid shrink-0 grid-rows-7 gap-1 pt-1 text-[10px] leading-3 text-muted-foreground">
          {heatmap.weekdayLabels.map((label) => (
            <div key={label} className="flex h-3 items-center">
              {label}
            </div>
          ))}
        </div>
        <div className="min-w-0 flex-1 overflow-x-auto pb-2">
          <div className="flex min-w-max gap-1 pt-1">
            {heatmap.weeks.map((week) => (
              <div key={week.weekIndex} className="grid grid-rows-7 gap-1">
                {week.days.map((day) => (
                  <HeatmapCell key={day.date} day={day} onActivate={setActiveDate} />
                ))}
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
        {activeDay ? (
          <div className="flex flex-col gap-1 md:flex-row md:items-center md:justify-between">
            <span>{activeDay.date}</span>
            <span className="tabular-nums">
              {activeDay.count} 场 / 当日输赢 <span className={amountTone(activeDay.resultAmount)}>{formatSigned(activeDay.resultAmount)}</span>
            </span>
          </div>
        ) : (
          "点击热力格查看当天记录"
        )}
      </div>
    </div>
  );
}

function HeatmapCell({
  day,
  onActivate,
}: {
  day: MahjongHeatmapDay;
  onActivate: (date: string) => void;
}) {
  if (!day.inRange) {
    return <div className="h-3 w-3 rounded-[3px]" />;
  }

  return (
    <button
      type="button"
      title={`${day.date}：${day.count} 场，${formatSigned(day.resultAmount)}`}
      aria-label={`${day.date}，${day.count} 场，当日输赢 ${formatSigned(day.resultAmount)}`}
      tabIndex={day.count > 0 ? 0 : -1}
      className="h-3 w-3 rounded-[3px] border border-border transition hover:ring-1 hover:ring-foreground focus:outline-none focus:ring-2 focus:ring-foreground"
      style={{ backgroundColor: getMahjongHeatmapCellColor(day), borderColor: day.count > 0 ? "#18181b" : undefined }}
      onClick={() => onActivate(day.date)}
      onFocus={() => onActivate(day.date)}
      onMouseEnter={() => onActivate(day.date)}
    />
  );
}

function HeatmapLegend() {
  return (
    <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
      <span className="inline-flex items-center gap-1">
        <span className="h-3 w-3 rounded-[3px] border border-border bg-white" />
        未去
      </span>
      <span className="inline-flex items-center gap-1">
        <span className="h-3 w-3 rounded-[3px] border border-foreground bg-foreground" />
        有去
      </span>
    </div>
  );
}

function FrequencyMetric({
  label,
  value,
  detail,
  icon: Icon,
}: {
  label: string;
  value: string;
  detail: string;
  icon: typeof CalendarDays;
}) {
  return (
    <div className="rounded-md border px-3 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">{label}</div>
        <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
      </div>
      <div className="mt-2 text-xl font-semibold tabular-nums">{value}</div>
      <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
    </div>
  );
}

function LegendDot({ color, label, value }: { color: string; label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: color }} />
      <span className="truncate">{label}</span>
      <span className="ml-auto tabular-nums text-foreground">{value}</span>
    </div>
  );
}

function ExtremeDay({ label, value, date, icon: Icon }: { label: string; value: string; date?: string | null; icon: typeof TrendingUp }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">{label}</div>
          <div className="text-xs text-muted-foreground">{date || "-"}</div>
        </div>
      </div>
      <div className={["tabular-nums text-sm font-semibold", amountTone(value)].join(" ")}>{formatSigned(value)}</div>
    </div>
  );
}

function buildTrendChart(data: MahjongTimelinePoint[]) {
  const values = data.flatMap((item) => [toNumber(item.result_amount), toNumber(item.cumulative_result), 0]);
  const domain = paddedDomain(values);
  const x = (index: number) => (data.length === 1 ? (chartLeft + chartRight) / 2 : chartLeft + (index / (data.length - 1)) * (chartRight - chartLeft));
  const y = (value: number) => chartBottom - ((value - domain.min) / (domain.max - domain.min || 1)) * (chartBottom - chartTop);
  const zeroY = y(0);
  const barWidth = Math.max(Math.min((chartRight - chartLeft) / Math.max(data.length, 1) * 0.42, 10), 3);
  const gridLines = Array.from({ length: 4 }, (_, index) => {
    const ratio = index / 3;
    return {
      y: chartTop + ratio * (chartBottom - chartTop),
      value: domain.max - ratio * (domain.max - domain.min),
    };
  });
  const bars = data.map((item, index) => {
    const value = toNumber(item.result_amount);
    const valueY = y(value);
    return {
      key: `bar-${item.played_date}-${index}`,
      x: x(index) - barWidth / 2,
      y: Math.min(valueY, zeroY),
      width: barWidth,
      height: Math.max(Math.abs(valueY - zeroY), 1),
      color: value >= 0 ? "#16a34a" : "#dc2626",
    };
  });
  const points = data.map((item, index) => ({
    key: `point-${item.played_date}-${index}`,
    x: x(index),
    y: y(toNumber(item.cumulative_result)),
  }));
  const linePath = points.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(" ");
  return { bars, points, linePath, gridLines, zeroY };
}

function paddedDomain(values: number[]) {
  const minValue = Math.min(...values);
  const maxValue = Math.max(...values);
  const padding = Math.max((maxValue - minValue) * 0.12, 20);
  return {
    min: minValue - padding,
    max: maxValue + padding,
  };
}

function getTodayText() {
  const now = new Date();
  const offset = now.getTimezoneOffset() * 60000;
  return new Date(now.getTime() - offset).toISOString().slice(0, 10);
}

function toNumber(value: string | number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatNumber(value: string | number) {
  return toNumber(value).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

function formatSigned(value: string | number) {
  const numeric = toNumber(value);
  const prefix = numeric > 0 ? "+" : "";
  return `${prefix}${formatNumber(numeric)}`;
}

function formatCompact(value: string | number) {
  return toNumber(value).toLocaleString("zh-CN", {
    maximumFractionDigits: 0,
  });
}

function amountTone(value: string | number) {
  const numeric = toNumber(value);
  if (numeric > 0) {
    return "text-emerald-700";
  }
  if (numeric < 0) {
    return "text-red-700";
  }
  return "text-foreground";
}

function amountSvgTone(value: string | number) {
  const numeric = toNumber(value);
  if (numeric > 0) {
    return "fill-emerald-700";
  }
  if (numeric < 0) {
    return "fill-red-700";
  }
  return "fill-foreground";
}
