import { useState } from "react";
import {
  CartesianGrid,
  Customized,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { cn } from "@/lib/utils";

export type TrendPoint = {
  label: string;
  primary: number;
  secondary: number;
};

type TrendChartProps = {
  data: TrendPoint[];
  primaryLabel: string;
  secondaryLabel: string;
  primaryColor?: string;
  secondaryColor?: string;
  ariaLabel: string;
  emptyLabel?: string;
};

type TooltipPayload = {
  payload?: TrendPoint;
};

type TooltipContentProps = {
  active?: boolean;
  label?: string | number;
  payload?: TooltipPayload[];
};

type ChartGeometry = {
  offset?: { left: number; top: number; width: number; height: number };
};

/** TrendChart 使用 Recharts 展示统一风格的按日趋势和十字准线。 */
export function TrendChart({
  data,
  primaryLabel,
  secondaryLabel,
  primaryColor = "#3b82f6",
  secondaryColor = "#14b8a6",
  ariaLabel,
  emptyLabel = "暂无数据",
}: TrendChartProps) {
  const [pointerY, setPointerY] = useState<number | null>(null);

  if (!data.length) {
    return (
      <div className="flex h-72 items-center justify-center text-sm text-muted-foreground">
        {emptyLabel}
      </div>
    );
  }

  return (
    <div className={cn("h-72 w-full")} role="img" aria-label={ariaLabel}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart
          data={data}
          margin={{ top: 12, right: 16, bottom: 4, left: 0 }}
          onMouseMove={(state) => {
            const y = state?.activeCoordinate?.y;
            setPointerY(typeof y === "number" ? y : null);
          }}
          onMouseLeave={() => setPointerY(null)}
        >
          <CartesianGrid
            vertical={false}
            stroke="currentColor"
            strokeOpacity={0.12}
          />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11, fill: "currentColor" }}
            minTickGap={28}
            interval="preserveStartEnd"
          />
          <YAxis
            allowDecimals={false}
            tickLine={false}
            axisLine={false}
            width={36}
            tick={{ fontSize: 11, fill: "currentColor" }}
          />
          <Tooltip
            cursor={{ stroke: "#94a3b8", strokeDasharray: "4 4", strokeOpacity: 0.55 }}
            content={<ChartTooltip primaryLabel={primaryLabel} secondaryLabel={secondaryLabel} />}
          />
          {pointerY != null && (
            <Customized
              component={(props: ChartGeometry) => {
                const offset = props.offset;
                if (!offset) return null;
                return (
                  <line
                    x1={offset.left}
                    x2={offset.left + offset.width}
                    y1={pointerY}
                    y2={pointerY}
                    stroke="#94a3b8"
                    strokeDasharray="4 4"
                    strokeOpacity={0.35}
                    pointerEvents="none"
                  />
                );
              }}
            />
          )}
          <Line
            type="monotone"
            dataKey="primary"
            name={primaryLabel}
            stroke={primaryColor}
            strokeWidth={2.5}
            dot={false}
            activeDot={{ r: 4, strokeWidth: 0 }}
            isAnimationActive={false}
          />
          <Line
            type="monotone"
            dataKey="secondary"
            name={secondaryLabel}
            stroke={secondaryColor}
            strokeWidth={2.5}
            dot={false}
            activeDot={{ r: 4, strokeWidth: 0 }}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
      <div className="mt-2 flex justify-center gap-6 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <span
            className="h-2 w-2 rounded-full"
            style={{ backgroundColor: primaryColor }}
          />
          {primaryLabel}
        </span>
        <span className="flex items-center gap-1.5">
          <span
            className="h-2 w-2 rounded-full"
            style={{ backgroundColor: secondaryColor }}
          />
          {secondaryLabel}
        </span>
      </div>
    </div>
  );
}

/** ChartTooltip 使用与 shadcn 卡片一致的浅色浮层展示当天数据。 */
function ChartTooltip({
  active,
  label,
  payload,
  primaryLabel,
  secondaryLabel,
}: TooltipContentProps & { primaryLabel: string; secondaryLabel: string }) {
  const point = payload?.[0]?.payload;
  if (!active || !point) return null;
  return (
    <div className="rounded-md border bg-background px-3 py-2 text-xs shadow-md">
      <div className="font-medium text-foreground">{label}</div>
      <div className="mt-1.5 space-y-1">
        <div className="flex items-center justify-between gap-6">
          <span className="text-muted-foreground">{primaryLabel}</span>
          <span className="font-semibold tabular-nums text-foreground">
            {point.primary}
          </span>
        </div>
        <div className="flex items-center justify-between gap-6">
          <span className="text-muted-foreground">{secondaryLabel}</span>
          <span className="font-semibold tabular-nums text-foreground">
            {point.secondary}
          </span>
        </div>
      </div>
    </div>
  );
}
