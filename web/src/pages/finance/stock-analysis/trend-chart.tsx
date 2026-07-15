import { useMemo } from "react";
import { LineChart } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatCompactMoney, formatPercent, maskSensitive, sensitiveMask, toNumber } from "./format";
import type { TimelinePoint, TrendSeries } from "./types";

const trendSeries: TrendSeries[] = [
  { key: "total_asset", label: "总资产", color: "#111827", axis: "money" },
  { key: "market_value", label: "总市值", color: "#2563eb", axis: "money" },
  { key: "daily_change", label: "记录间资产变化", color: "#059669", axis: "money" },
  { key: "position_percent", label: "总仓位", color: "#d97706", axis: "percent" },
];

export function CombinedTrendCard({ data, isSensitiveMasked }: { data: TimelinePoint[]; isSensitiveMasked: boolean }) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>综合趋势</CardTitle>
            <CardDescription>
              {isSensitiveMasked
                ? "金额曲线默认隐藏，只展示总仓位变化。"
                : "总资产、总市值、记录间资产变化和总仓位放在同一张图。左轴为金额，右轴为仓位。"}
            </CardDescription>
          </div>
          <LineChart className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
      </CardHeader>
      <CardContent>
        <CombinedTrendSvg data={data} isSensitiveMasked={isSensitiveMasked} />
      </CardContent>
    </Card>
  );
}

function CombinedTrendSvg({ data, isSensitiveMasked }: { data: TimelinePoint[]; isSensitiveMasked: boolean }) {
  const activeTrendSeries = useMemo(
    () => (isSensitiveMasked ? trendSeries.filter((item) => item.axis === "percent") : trendSeries),
    [isSensitiveMasked],
  );
  const chart = useMemo(() => buildCombinedChart(data, activeTrendSeries), [data, activeTrendSeries]);
  const latest = data[data.length - 1];

  return (
    <div className="space-y-3">
      <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-2 xl:grid-cols-4">
        {trendSeries.map((item) => (
          <div key={item.label} className="flex min-w-0 items-center gap-2">
            <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: item.color }} />
            <span className="truncate">{item.label}</span>
            <span className="ml-auto tabular-nums text-foreground">
              {item.axis === "percent" ? formatPercent(latest[item.key]) : maskSensitive(isSensitiveMasked, formatCompactMoney(latest[item.key]))}
            </span>
          </div>
        ))}
      </div>
      <svg viewBox="0 0 760 280" className="h-72 w-full" role="img" aria-label="综合趋势图">
        {chart.gridLines.map((line) => (
          <g key={line.y}>
            <line x1="60" x2="700" y1={line.y} y2={line.y} stroke="#e5e7eb" strokeWidth="1" />
            <text x="52" y={line.y + 4} textAnchor="end" className="fill-muted-foreground text-[10px]">
              {isSensitiveMasked ? sensitiveMask : formatCompactMoney(line.moneyValue)}
            </text>
            <text x="708" y={line.y + 4} className="fill-muted-foreground text-[10px]">
              {formatPercent(line.percentValue)}
            </text>
          </g>
        ))}
        <line x1="60" x2="700" y1="226" y2="226" stroke="#d4d4d8" />
        {chart.paths.map((path) => (
          <path key={path.label} d={path.d} fill="none" stroke={path.color} strokeLinecap="round" strokeLinejoin="round" strokeWidth="2.5" />
        ))}
        {chart.points.map((point) => (
          <circle key={`${point.label}-${point.x}-${point.y}`} cx={point.x} cy={point.y} r="3" fill={point.color} />
        ))}
        <text x="60" y="254" className="fill-muted-foreground text-[10px]">
          {data[0].snapshot_date}
        </text>
        <text x="700" y="254" textAnchor="end" className="fill-muted-foreground text-[10px]">
          {data[data.length - 1].snapshot_date}
        </text>
      </svg>
    </div>
  );
}

function buildCombinedChart(data: TimelinePoint[], seriesList: TrendSeries[]) {
  const left = 60;
  const right = 700;
  const top = 18;
  const bottom = 226;
  const moneyKeys = seriesList.filter((item) => item.axis === "money").map((item) => item.key);
  const percentKeys = seriesList.filter((item) => item.axis === "percent").map((item) => item.key);
  const moneyValues = data.flatMap((item) => moneyKeys.map((key) => toNumber(item[key]))).concat(0);
  const percentValues = data.flatMap((item) => percentKeys.map((key) => toNumber(item[key])));
  const moneyDomain = paddedDomain(moneyValues, 1);
  const percentDomain = paddedDomain(percentValues, 3, 0, 100);
  const x = (index: number) => (data.length === 1 ? (left + right) / 2 : left + (index / (data.length - 1)) * (right - left));
  const moneyY = (value: number) => bottom - ((value - moneyDomain.min) / (moneyDomain.max - moneyDomain.min || 1)) * (bottom - top);
  const percentY = (value: number) => bottom - ((value - percentDomain.min) / (percentDomain.max - percentDomain.min || 1)) * (bottom - top);
  const gridLines = Array.from({ length: 4 }, (_, index) => {
    const ratio = index / 3;
    return {
      y: top + ratio * (bottom - top),
      moneyValue: moneyDomain.max - ratio * (moneyDomain.max - moneyDomain.min),
      percentValue: percentDomain.max - ratio * (percentDomain.max - percentDomain.min),
    };
  });
  const paths = seriesList.map((series) => ({
    label: series.label,
    color: series.color,
    d: data
      .map((item, index) => {
        const y = series.axis === "percent" ? percentY(toNumber(item[series.key])) : moneyY(toNumber(item[series.key]));
        return `${index === 0 ? "M" : "L"} ${x(index).toFixed(2)} ${y.toFixed(2)}`;
      })
      .join(" "),
  }));
  const points = data.flatMap((item, index) =>
    seriesList.map((series) => {
      const y = series.axis === "percent" ? percentY(toNumber(item[series.key])) : moneyY(toNumber(item[series.key]));
      return { label: series.label, color: series.color, x: x(index), y };
    }),
  );

  return { gridLines, paths, points };
}

function paddedDomain(values: number[], minimumPadding: number, hardMin?: number, hardMax?: number) {
  const finiteValues = values.filter(Number.isFinite);
  const rawMin = finiteValues.length ? Math.min(...finiteValues) : 0;
  const rawMax = finiteValues.length ? Math.max(...finiteValues) : 1;
  const padding = Math.max((rawMax - rawMin) * 0.1, minimumPadding);
  let min = rawMin - padding;
  let max = rawMax + padding;
  if (hardMin !== undefined) {
    min = Math.max(hardMin, min);
  }
  if (hardMax !== undefined) {
    max = Math.min(hardMax, max);
  }
  if (min === max) {
    max = min + minimumPadding * 2;
  }
  return { min, max };
}
