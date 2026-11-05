import { useState, type PointerEvent } from "react";

import type { ReceiptDay } from "@/lib/receipt-split";

const WIDTH = 980;
const HEIGHT = 260;
const LEFT = 50;
const RIGHT = 950;
const TOP = 20;
const BOTTOM = 215;
const TOOLTIP_WIDTH = 168;

type Hover = { index: number; y: number };

/** ReceiptTrend 展示每日识别用户和生成账单用户，并跟随鼠标显示十字定位。 */
export function ReceiptTrend({ days }: { days: ReceiptDay[] }) {
  const [hover, setHover] = useState<Hover | null>(null);
  const max = Math.max(
    1,
    ...days.flatMap((day) => [day.analysis_users, day.bill_users]),
  );
  const x = (index: number) =>
    LEFT + (index * (RIGHT - LEFT)) / Math.max(1, days.length - 1);
  const y = (value: number) => BOTTOM - (value / max) * (BOTTOM - TOP);
  const activeDay = hover ? days[hover.index] : null;
  const activeX = hover ? x(hover.index) : 0;
  const tooltipX = hover
    ? activeX > RIGHT - TOOLTIP_WIDTH - 24
      ? activeX - TOOLTIP_WIDTH - 12
      : activeX + 12
    : 0;

  /** move 把指针映射到最近日期，并记录十字准线的纵向位置。 */
  function move(event: PointerEvent<SVGSVGElement>) {
    if (!days.length) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const viewX = ((event.clientX - rect.left) / (rect.width || WIDTH)) * WIDTH;
    const viewY = ((event.clientY - rect.top) / (rect.height || HEIGHT)) * HEIGHT;
    const ratio = (viewX - LEFT) / (RIGHT - LEFT);
    const index = Math.max(
      0,
      Math.min(days.length - 1, Math.round(ratio * (days.length - 1))),
    );
    setHover({
      index,
      y: Math.max(TOP, Math.min(BOTTOM, viewY)),
    });
  }

  return (
    <>
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="w-full"
        role="img"
        aria-label="每日识别用户与生成账单用户趋势"
        style={{ touchAction: "none" }}
        onPointerMove={move}
        onPointerLeave={() => setHover(null)}
      >
        {[0, 0.5, 1].map((ratio) => (
          <g key={ratio}>
            <line
              x1={LEFT}
              x2={RIGHT}
              y1={y(max * ratio)}
              y2={y(max * ratio)}
              stroke="currentColor"
              opacity=".1"
            />
            <text
              x="40"
              y={y(max * ratio) + 4}
              textAnchor="end"
              fontSize="11"
              fill="currentColor"
            >
              {(max * ratio).toLocaleString("zh-CN", {
                maximumFractionDigits: 1,
              })}
            </text>
          </g>
        ))}
        {(["analysis_users", "bill_users"] as const).map((key, seriesIndex) => (
          <g key={key}>
            <polyline
              points={days.map((day, index) => `${x(index)},${y(day[key])}`).join(" ")}
              stroke={seriesIndex ? "#14b8a6" : "#3b82f6"}
              strokeWidth="2.5"
              fill="none"
            />
            {days.map((day, index) => (
              <circle
                key={day.date}
                cx={x(index)}
                cy={y(day[key])}
                r={days.length > 90 ? 1 : 3}
                fill={seriesIndex ? "#14b8a6" : "#3b82f6"}
              />
            ))}
          </g>
        ))}
        {hover && activeDay && (
          <g pointerEvents="none">
            <line
              x1={activeX}
              x2={activeX}
              y1={TOP}
              y2={BOTTOM}
              stroke="currentColor"
              strokeOpacity=".35"
              strokeDasharray="4 4"
            />
            <line
              x1={LEFT}
              x2={RIGHT}
              y1={hover.y}
              y2={hover.y}
              stroke="currentColor"
              strokeOpacity=".2"
              strokeDasharray="4 4"
            />
            <circle
              cx={activeX}
              cy={y(activeDay.analysis_users)}
              r="4"
              fill="#3b82f6"
            />
            <circle
              cx={activeX}
              cy={y(activeDay.bill_users)}
              r="4"
              fill="#14b8a6"
            />
            <g transform={`translate(${tooltipX}, ${TOP + 6})`}>
              <rect
                width={TOOLTIP_WIDTH}
                height="64"
                rx="6"
                fill="#ffffff"
                stroke="#cbd5e1"
                strokeWidth="1"
              />
              <text x="10" y="17" fontSize="11" fontWeight="600" fill="#0f172a">
                {activeDay.date}
              </text>
              <text x="10" y="37" fontSize="11" fill="#475569">
                识别用户{" "}
                <tspan fill="#2563eb" fontWeight="600">
                  {activeDay.analysis_users}
                </tspan>
              </text>
              <text x="10" y="54" fontSize="11" fill="#475569">
                生成账单用户{" "}
                <tspan fill="#0f766e" fontWeight="600">
                  {activeDay.bill_users}
                </tspan>
              </text>
            </g>
          </g>
        )}
        {days
          .filter(
            (_, index) =>
              index === 0 ||
              index === days.length - 1 ||
              index === Math.floor(days.length / 2),
          )
          .map((day) => (
            <text
              key={day.date}
              x={x(days.indexOf(day))}
              y="246"
              textAnchor={
                day === days[0]
                  ? "start"
                  : day === days[days.length - 1]
                    ? "end"
                    : "middle"
              }
              fontSize="11"
              fill="currentColor"
            >
              {day.date}
            </text>
          ))}
      </svg>
      <div className="flex justify-center gap-6 text-xs">
        <span className="text-blue-500">● 识别用户</span>
        <span className="text-teal-500">● 生成账单用户</span>
      </div>
    </>
  );
}
