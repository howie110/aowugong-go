import { useMemo, useState, type PointerEvent } from "react";
import { LineChart } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatCompactMoney, formatPercent, maskSensitive, toNumber } from "./format";
import type { TimelinePoint, TrendSeries } from "./types";

const chartFrame = { width: 760, height: 280, left: 104, right: 650, top: 18, bottom: 226 };
const axisX = { money: 96, change: 658, percent: 716 };
const tooltipFrame = { width: 236, height: 100 };

const trendSeries: TrendSeries[] = [
  { key: "total_asset", label: "总资产", color: "#111827", axis: "money" },
  { key: "market_value", label: "总市值", color: "#2563eb", axis: "money" },
  { key: "daily_change", label: "资产变化额", color: "#059669", axis: "change" },
  { key: "position_percent", label: "总仓位", color: "#d97706", axis: "percent" },
];

// CombinedTrendCard 展示综合趋势卡片，并把仓位时间线传给交互图表。
// 输入：data 是按日期升序排列的仓位时间线，isSensitiveMasked 控制金额脱敏。
// 输出：返回综合趋势卡片。
// 副作用：无。
export function CombinedTrendCard({ data, isSensitiveMasked }: { data: TimelinePoint[]; isSensitiveMasked: boolean }) {
  // 1. 渲染标题、说明和交互趋势图。
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>综合趋势</CardTitle>
            <CardDescription>
              {isSensitiveMasked
                ? "金额曲线默认隐藏，只展示总仓位变化。"
                : "总资产、总市值、资产变化额和总仓位放在同一张图，并分别使用对应刻度。"}
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

// CombinedTrendSvg 绘制综合趋势，并按麻将趋势的交互方式跟随指针显示准星和浮层。
// 输入：data 是仓位时间线，isSensitiveMasked 控制金额及其变化值是否隐藏。
// 输出：返回可跟随指针的 SVG 图表和左侧精简图例。
// 副作用：移动指针会显示当前记录，移出图表后隐藏准星和浮层。
function CombinedTrendSvg({ data, isSensitiveMasked }: { data: TimelinePoint[]; isSensitiveMasked: boolean }) {
  // 1. 根据脱敏和图例开关准备曲线，并记录当前悬停的数据索引。
  const [hiddenSeries, setHiddenSeries] = useState<Set<TrendSeries["key"]>>(new Set());
  const activeTrendSeries = useMemo(
    () => trendSeries.filter((item) => (!isSensitiveMasked || item.axis === "percent") && !hiddenSeries.has(item.key)),
    [hiddenSeries, isSensitiveMasked],
  );
  const chart = useMemo(() => buildCombinedChart(data, activeTrendSeries), [data, activeTrendSeries]);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);

  if (!data.length) {
    return null;
  }

  // 2. 仅在悬停时同步当前记录、主曲线点和浮层位置。
  const activeIndex = hoveredIndex;
  const active = activeIndex === null ? null : data[activeIndex];
  const assetChangePercent = active ? formatAssetChangePercent(active.total_asset, active.daily_change) : "--";
  const activePoints = activeIndex === null ? [] : chart.points.filter((point) => point.index === activeIndex);
  const activeChartPoint = activePoints[0];
  const visibleAxes = new Set(activeTrendSeries.map((item) => item.axis));
  const tooltipX = activeChartPoint
    ? Math.min(Math.max(activeChartPoint.x + 14, chartFrame.left + 8), chartFrame.right - tooltipFrame.width - 8)
    : chartFrame.left + 8;
  const tooltipY = activeChartPoint
    ? Math.min(Math.max(activeChartPoint.y - 50, chartFrame.top + 8), chartFrame.bottom - tooltipFrame.height - 8)
    : chartFrame.top + 8;

  // handlePointerMove 根据指针横向位置吸附到最近的一条日期记录。
  // 输入：event 是覆盖绘图区的透明 SVG 矩形指针事件。
  // 输出：无。
  // 副作用：更新悬停索引，驱动左侧指标、十字准星和浮层同步变化。
  function handlePointerMove(event: PointerEvent<SVGRectElement>) {
    // 1. 使用透明绘图区的实际屏幕宽度计算指针比例。
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);

    // 2. 将比例映射到最近的时间线索引。
    setHoveredIndex(Math.round(ratio * (data.length - 1)));
  }

  // toggleSeries 切换指定趋势曲线及其坐标轴的显示状态。
  // 输入：key 是趋势系列字段名。
  // 输出：无。
  // 副作用：更新隐藏系列集合并触发图表重绘。
  function toggleSeries(key: TrendSeries["key"]) {
    // 1. 复制当前集合，并按现有状态增加或移除系列字段。
    setHiddenSeries((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  // 3. 左侧展示精简图例，右侧集中展示曲线、准星及完整数据浮层。
  return (
    <div className="grid gap-2 md:grid-cols-[7rem_minmax(0,1fr)] md:items-stretch">
      <ul className="grid list-none grid-cols-2 gap-x-3 gap-y-2 md:flex md:flex-col md:items-start md:justify-center">
        {trendSeries.map((item) => {
          const isAvailable = !isSensitiveMasked || item.axis === "percent";
          const isVisible = isAvailable && !hiddenSeries.has(item.key);
          const actionLabel = `${isVisible ? "隐藏" : "显示"}${item.label}曲线`;
          return (
            <li key={item.label} className="flex items-center gap-1.5">
              <button
                type="button"
                aria-label={actionLabel}
                aria-pressed={isVisible}
                title={actionLabel}
                disabled={!isAvailable}
                onClick={() => toggleSeries(item.key)}
                className={[
                  "flex h-7 w-7 shrink-0 items-center justify-center rounded border transition-colors",
                  isAvailable
                    ? "hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    : "cursor-not-allowed opacity-30",
                ].join(" ")}
              >
                <span
                  aria-hidden="true"
                  className="h-2.5 w-2.5 shrink-0 rounded-full border"
                  style={{ backgroundColor: isVisible ? item.color : "transparent", borderColor: item.color }}
                />
              </button>
              <span className={["whitespace-nowrap text-xs", isVisible ? "text-muted-foreground" : "text-muted-foreground/50"].join(" ")}>
                {item.label}
              </span>
            </li>
          );
        })}
      </ul>

      <svg
        viewBox={`0 0 ${chartFrame.width} ${chartFrame.height}`}
        className="h-72 w-full touch-none select-none"
        role="img"
        aria-label="股票仓位综合趋势图"
      >
        {chart.gridLines.map((line) => (
          <g key={line.y}>
            <line x1={chartFrame.left} x2={chartFrame.right} y1={line.y} y2={line.y} stroke="#e5e7eb" strokeWidth="1" />
            {visibleAxes.has("money") ? (
              <text x={axisX.money} y={line.y + 4} textAnchor="end" fill={trendSeries[0].color} className="text-[10px]">
                {formatCompactMoney(line.moneyValue)}
              </text>
            ) : null}
            {visibleAxes.has("change") ? (
              <text x={axisX.change} y={line.y + 4} fill={trendSeries[2].color} className="text-[10px]">
                {formatSignedCompactMoney(line.changeValue)}
              </text>
            ) : null}
            {visibleAxes.has("percent") ? (
              <text x={axisX.percent} y={line.y + 4} fill={trendSeries[3].color} className="text-[10px]">
                {formatPercent(line.percentValue)}
              </text>
            ) : null}
          </g>
        ))}
        <line x1={chartFrame.left} x2={chartFrame.right} y1={chartFrame.bottom} y2={chartFrame.bottom} stroke="#d4d4d8" />
        {chart.paths.map((path) => (
          <path
            key={path.label}
            d={path.d}
            fill="none"
            stroke={path.color}
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="2.5"
          />
        ))}
        {chart.points.map((point) => (
          <circle
            key={`${point.label}-${point.x}-${point.y}`}
            cx={point.x}
            cy={point.y}
            r={point.index === activeIndex ? "5" : "3"}
            fill={point.color}
          />
        ))}
        {activeChartPoint && active ? (
          <g pointerEvents="none">
            <line
              x1={activeChartPoint.x}
              x2={activeChartPoint.x}
              y1={chartFrame.top}
              y2={chartFrame.bottom}
              stroke="#111827"
              strokeDasharray="4 4"
              opacity="0.55"
            />
            <line
              x1={chartFrame.left}
              x2={chartFrame.right}
              y1={activeChartPoint.y}
              y2={activeChartPoint.y}
              stroke="#111827"
              strokeDasharray="4 4"
              opacity="0.25"
            />
            <rect x={tooltipX} y={tooltipY} width={tooltipFrame.width} height={tooltipFrame.height} rx="6" fill="#ffffff" stroke="#d4d4d8" />
            <text x={tooltipX + 12} y={tooltipY + 18} className="fill-foreground text-[12px] font-semibold">
              {active.snapshot_date}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 37} fill={trendSeries[0].color} className="text-[10px]">
              总资产：{maskSensitive(isSensitiveMasked, formatCompactMoney(active.total_asset))}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 54} fill={trendSeries[1].color} className="text-[10px]">
              总市值：{maskSensitive(isSensitiveMasked, formatCompactMoney(active.market_value))}
            </text>
            <text x={tooltipX + 12} y={tooltipY + 71} fill={trendSeries[2].color} className="text-[10px]">
              资产变化：
              <tspan className={svgChangeTone(active.daily_change, isSensitiveMasked)}>
                {maskSensitive(isSensitiveMasked, formatSignedCompactMoney(active.daily_change))}（{maskSensitive(isSensitiveMasked, assetChangePercent)}）
              </tspan>
            </text>
            <text x={tooltipX + 12} y={tooltipY + 88} fill={trendSeries[3].color} className="text-[10px]">
              总仓位：{formatPercent(active.position_percent)}
            </text>
          </g>
        ) : null}
        <text x={chartFrame.left} y="254" className="fill-muted-foreground text-[10px]">
          {data[0].snapshot_date}
        </text>
        <text x={chartFrame.right} y="254" textAnchor="end" className="fill-muted-foreground text-[10px]">
          {data[data.length - 1].snapshot_date}
        </text>
        <rect
          x={chartFrame.left}
          y={chartFrame.top}
          width={chartFrame.right - chartFrame.left}
          height={chartFrame.bottom - chartFrame.top}
          fill="transparent"
          onPointerMove={handlePointerMove}
          onPointerLeave={() => setHoveredIndex(null)}
        />
      </svg>
    </div>
  );
}

// formatSignedCompactMoney 格式化带正负号的紧凑金额。
// 输入：value 是金额差额。
// 输出：返回带正号、负号和万元单位的字符串。
// 副作用：无。
function formatSignedCompactMoney(value: string | number) {
  // 1. 正数补充正号，其余沿用金额格式中的符号。
  const numberValue = toNumber(value);
  return `${numberValue > 0 ? "+" : ""}${formatCompactMoney(numberValue)}`;
}

// calculateAssetChangePercent 计算本次资产变化相对上次总资产的百分比。
// 输入：totalAsset 是本次总资产，dailyChange 是相对上次记录的资产变化额。
// 输出：返回变化百分比；无法得到有效上次资产时返回 null。
// 副作用：无。
function calculateAssetChangePercent(totalAsset: string | number, dailyChange: string | number) {
  // 1. 使用本次总资产减去变化额，还原上次总资产。
  const changeValue = toNumber(dailyChange);
  const previousAsset = toNumber(totalAsset) - changeValue;
  if (previousAsset === 0) {
    return null;
  }

  // 2. 使用上次总资产绝对值作为基数计算变化率。
  return (changeValue / Math.abs(previousAsset)) * 100;
}

// formatSignedPercent 格式化带正负号的百分比。
// 输入：value 是百分比数值。
// 输出：返回带正号、负号和两位小数的百分比。
// 副作用：无。
function formatSignedPercent(value: string | number) {
  // 1. 正数补充正号，其余保留原有符号。
  const numberValue = toNumber(value);
  return `${numberValue > 0 ? "+" : ""}${numberValue.toFixed(2)}%`;
}

// formatAssetChangePercent 计算并格式化总资产相对上次记录的变化百分比。
// 输入：totalAsset 是本次总资产，dailyChange 是相对上次记录的资产变化额。
// 输出：返回带正负号的百分比；无法计算时返回 --。
// 副作用：无。
function formatAssetChangePercent(totalAsset: string | number, dailyChange: string | number) {
  // 1. 复用统一变化率计算并处理无有效分母的情况。
  const percent = calculateAssetChangePercent(totalAsset, dailyChange);
  return percent === null ? "--" : formatSignedPercent(percent);
}


// svgChangeTone 根据资产变化为 SVG 浮层文字选择颜色。
// 输入：value 是资产变化，isSensitiveMasked 表示金额是否脱敏。
// 输出：返回 Tailwind SVG 填充色类名。
// 副作用：无。
function svgChangeTone(value: string | number, isSensitiveMasked: boolean) {
  // 1. 脱敏时使用中性色，避免颜色间接暴露金额方向。
  if (isSensitiveMasked) {
    return "fill-foreground";
  }

  // 2. 按国内行情习惯使用红涨绿跌，零变化使用正文色。
  const numberValue = toNumber(value);
  if (numberValue > 0) {
    return "fill-red-700";
  }
  if (numberValue < 0) {
    return "fill-emerald-700";
  }
  return "fill-foreground";
}

// buildCombinedChart 把仓位时间线转换成坐标轴、曲线路径和数据点。
// 输入：data 是时间线数据，seriesList 是当前需要绘制的系列。
// 输出：返回网格、路径和带记录索引的数据点。
// 副作用：无。
function buildCombinedChart(data: TimelinePoint[], seriesList: TrendSeries[]) {
  // 1. 分别计算总金额、资产变化额和仓位轴的安全范围。
  const moneyKeys = seriesList.filter((item) => item.axis === "money").map((item) => item.key);
  const changeKeys = seriesList.filter((item) => item.axis === "change").map((item) => item.key);
  const percentKeys = seriesList.filter((item) => item.axis === "percent").map((item) => item.key);
  const moneyValues = data.flatMap((item) => moneyKeys.map((key) => toNumber(item[key]))).concat(0);
  const changeValues = data.flatMap((item) => changeKeys.map((key) => toNumber(item[key]))).concat(0);
  const percentValues = data.flatMap((item) => percentKeys.map((key) => toNumber(item[key])));
  const moneyDomain = paddedDomain(moneyValues, 1);
  const changeDomain = paddedDomain(changeValues, 1);
  const percentDomain = paddedDomain(percentValues, 3, 0, 100);
  const x = (index: number) =>
    data.length === 1
      ? (chartFrame.left + chartFrame.right) / 2
      : chartFrame.left + (index / (data.length - 1)) * (chartFrame.right - chartFrame.left);
  const moneyY = (value: number) =>
    chartFrame.bottom -
    ((value - moneyDomain.min) / (moneyDomain.max - moneyDomain.min || 1)) * (chartFrame.bottom - chartFrame.top);
  const changeY = (value: number) =>
    chartFrame.bottom -
    ((value - changeDomain.min) / (changeDomain.max - changeDomain.min || 1)) * (chartFrame.bottom - chartFrame.top);
  const percentY = (value: number) =>
    chartFrame.bottom -
    ((value - percentDomain.min) / (percentDomain.max - percentDomain.min || 1)) * (chartFrame.bottom - chartFrame.top);

  // 2. 生成双轴共用的水平网格和曲线路径。
  const gridLines = Array.from({ length: 4 }, (_, index) => {
    const ratio = index / 3;
    return {
      y: chartFrame.top + ratio * (chartFrame.bottom - chartFrame.top),
      moneyValue: moneyDomain.max - ratio * (moneyDomain.max - moneyDomain.min),
      changeValue: changeDomain.max - ratio * (changeDomain.max - changeDomain.min),
      percentValue: percentDomain.max - ratio * (percentDomain.max - percentDomain.min),
    };
  });
  const paths = seriesList.map((series) => ({
    label: series.label,
    color: series.color,
    d: data
      .map((item, index) => {
        const y =
          series.axis === "percent"
            ? percentY(toNumber(item[series.key]))
            : series.axis === "change"
              ? changeY(toNumber(item[series.key]))
              : moneyY(toNumber(item[series.key]));
        return `${index === 0 ? "M" : "L"} ${x(index).toFixed(2)} ${y.toFixed(2)}`;
      })
      .join(" "),
  }));

  // 3. 保留点所属记录索引，供选中日期时高亮。
  const points = data.flatMap((item, index) =>
    seriesList.map((series) => {
      const y =
        series.axis === "percent"
          ? percentY(toNumber(item[series.key]))
          : series.axis === "change"
            ? changeY(toNumber(item[series.key]))
            : moneyY(toNumber(item[series.key]));
      return { label: series.label, color: series.color, index, x: x(index), y };
    }),
  );
  return { gridLines, paths, points };
}

// paddedDomain 为数据轴增加留白，并可限制硬边界。
// 输入：values 是轴数据，minimumPadding 是最小留白，hardMin 和 hardMax 是可选边界。
// 输出：返回轴的最小值和最大值。
// 副作用：无。
function paddedDomain(values: number[], minimumPadding: number, hardMin?: number, hardMax?: number) {
  // 1. 清理非有限数值并计算原始范围。
  const finiteValues = values.filter(Number.isFinite);
  const rawMin = finiteValues.length ? Math.min(...finiteValues) : 0;
  const rawMax = finiteValues.length ? Math.max(...finiteValues) : 1;
  const padding = Math.max((rawMax - rawMin) * 0.1, minimumPadding);
  let min = rawMin - padding;
  let max = rawMax + padding;

  // 2. 应用硬边界并避免零跨度坐标轴。
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
