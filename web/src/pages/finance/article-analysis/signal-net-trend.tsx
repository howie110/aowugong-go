import { useEffect, useMemo, useRef, useState, type PointerEvent } from "react";

import type { SignalNetPoint } from "@/lib/article-analysis";

const minimumWidth = 480;
const frameHeight = 104;
const tooltip = { width: 112, height: 38 };

/** SignalNetTrend 绘制概念组每日推荐减风险净数，并在悬停时展示日期和数值。 */
export function SignalNetTrend({ points }: { points: SignalNetPoint[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [chartWidth, setChartWidth] = useState(minimumWidth);
  const frame = useMemo(
    () => ({ width: chartWidth, height: frameHeight, left: 30, right: chartWidth - 10, top: 8, bottom: 72 }),
    [chartWidth],
  );
  const chart = useMemo(() => buildSignalNetChart(points, frame), [frame, points]);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const active = hoveredIndex === null ? null : chart.points[hoveredIndex];
  const tooltipX = active ? Math.min(Math.max(active.x + 10, frame.left + 4), frame.right - tooltip.width) : frame.left;
  const tooltipY = active ? (active.y > frame.top + 45 ? active.y - tooltip.height - 6 : active.y + 6) : frame.top;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }
    const updateWidth = () => setChartWidth(Math.max(minimumWidth, Math.round(container.getBoundingClientRect().width)));
    updateWidth();
    const observer = new ResizeObserver(updateWidth);
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  if (!points.length) {
    return <div className="flex h-24 items-center text-xs text-muted-foreground">暂无趋势</div>;
  }

  function handlePointerMove(event: PointerEvent<SVGRectElement>) {
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    setHoveredIndex(Math.round(ratio * (points.length - 1)));
  }

  return (
    <div ref={containerRef} className="w-full">
      <svg
        viewBox={`0 0 ${frame.width} ${frame.height}`}
        className="block h-[6.5rem] w-full touch-none select-none"
        role="img"
        aria-label="概念组每日净数变化图"
      >
        <line x1={frame.left} x2={frame.right} y1={chart.zeroY} y2={chart.zeroY} stroke="#d4d4d8" strokeDasharray="3 3" />
        <text x={frame.left - 5} y={frame.top + 4} textAnchor="end" className="fill-muted-foreground text-[9px]">
          {formatSigned(chart.max)}
        </text>
        <text x={frame.left - 5} y={frame.bottom + 3} textAnchor="end" className="fill-muted-foreground text-[9px]">
          {formatSigned(chart.min)}
        </text>
        <path d={chart.path} fill="none" stroke="#52525b" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        {active ? (
          <g pointerEvents="none">
            <line x1={active.x} x2={active.x} y1={frame.top} y2={frame.bottom} stroke="#71717a" strokeDasharray="3 3" />
            <line x1={frame.left} x2={frame.right} y1={active.y} y2={active.y} stroke="#a1a1aa" strokeDasharray="3 3" />
            <circle cx={active.x} cy={active.y} r="4" fill={pointColor(active.net_count)} />
            <rect x={tooltipX} y={tooltipY} width={tooltip.width} height={tooltip.height} rx="5" fill="#ffffff" stroke="#d4d4d8" />
            <text x={tooltipX + 9} y={tooltipY + 15} className="fill-foreground text-[10px] font-medium">
              {active.date}
            </text>
            <text x={tooltipX + 9} y={tooltipY + 30} fill={pointColor(active.net_count)} className="text-[10px] font-semibold">
              净数 {formatSigned(active.net_count)}
            </text>
          </g>
        ) : null}
        <text x={frame.left} y="94" className="fill-muted-foreground text-[9px]">{shortDate(points[0].date)}</text>
        <text x={frame.right} y="94" textAnchor="end" className="fill-muted-foreground text-[9px]">
          {shortDate(points[points.length - 1].date)}
        </text>
        <rect
          x={frame.left}
          y={frame.top}
          width={frame.right - frame.left}
          height={frame.bottom - frame.top}
          fill="transparent"
          onPointerMove={handlePointerMove}
          onPointerLeave={() => setHoveredIndex(null)}
        />
      </svg>
    </div>
  );
}

function buildSignalNetChart(
  points: SignalNetPoint[],
  frame: { left: number; right: number; top: number; bottom: number },
) {
  const values = points.map((point) => point.net_count).concat(0);
  let min = Math.min(...values);
  let max = Math.max(...values);
  min -= 1;
  max += 1;
  const x = (index: number) =>
    points.length === 1 ? (frame.left + frame.right) / 2 : frame.left + (index / (points.length - 1)) * (frame.right - frame.left);
  const y = (value: number) => frame.bottom - ((value - min) / (max - min)) * (frame.bottom - frame.top);
  const chartPoints = points.map((point, index) => ({ ...point, x: x(index), y: y(point.net_count) }));
  return {
    min,
    max,
    zeroY: y(0),
    points: chartPoints,
    path: chartPoints.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(" "),
  };
}

function pointColor(value: number) {
  return value > 0 ? "#b91c1c" : value < 0 ? "#047857" : "#52525b";
}

function formatSigned(value: number) {
  return value > 0 ? `+${value}` : String(value);
}

function shortDate(value: string) {
  return value.length >= 10 ? value.slice(5, 10) : value;
}
