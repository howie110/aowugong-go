import { TrendChart } from "@/components/charts/trend-chart";
import type { AnalyticsReport } from "@/lib/analytics";
import { formatBeijingDate } from "@/lib/beijing-time";

import world from "./world.json";

export const number = (n: number) =>
  n.toLocaleString("zh-CN", { maximumFractionDigits: 1 });

/** Trend 展示最近 30 天的每日浏览量和访客，并跟随鼠标显示十字定位。 */
export function Trend({ data }: { data: AnalyticsReport["trend"] }) {
  return (
    <TrendChart
      data={data.map((point) => ({
        label: formatBeijingDate(point.time),
        primary: point.views,
        secondary: point.visitors,
      }))}
      primaryLabel="浏览量"
      secondaryLabel="访客"
      primaryColor="#3b82f6"
      secondaryColor="#06b6d4"
      ariaLabel="最近 30 天每日浏览量趋势"
      emptyLabel="暂无访问记录"
    />
  );
}

export function Heatmap({ data }: { data: AnalyticsReport["heat"] }) {
  const max = Math.max(1, ...data.map((x) => x.count));
  const values = new Map(data.map((x) => [`${x.day}-${x.hour}`, x.count]));
  return (
    <div className="grid grid-cols-8 gap-1 text-[10px]">
      <span />
      {["一", "二", "三", "四", "五", "六", "日"].map((d) => (
        <span key={d} className="text-center text-muted-foreground">
          周{d}
        </span>
      ))}
      {Array.from({ length: 24 }, (_, hour) => (
        <div key={hour} className="contents">
          <span className="text-right text-muted-foreground">{hour}:00</span>
          {Array.from({ length: 7 }, (_, i) => {
            const n = values.get(`${i + 1}-${hour}`) || 0;
            return (
              <div
                key={i}
                className="h-3.5 rounded-sm bg-muted"
                style={
                  n
                    ? {
                        backgroundColor: `rgba(59,130,246,${0.18 + (0.82 * n) / max})`,
                      }
                    : undefined
                }
                title={`周${i + 1} ${hour}:00 · ${n} 次浏览`}
                aria-label={`周${i + 1} ${hour}点 ${n}次浏览`}
              />
            );
          })}
        </div>
      ))}
    </div>
  );
}

// Natural Earth 1:110m 国界，公共领域；原始来源与转换说明见 README。
export function WorldMap({
  data,
  onCountry,
}: {
  data: AnalyticsReport["ranks"][string];
  onCountry: (country: string) => void;
}) {
  const counts = new Map(data.map((r) => [r.name, r.count]));
  const max = Math.max(1, ...data.map((r) => r.count));
  return (
    <svg
      viewBox="0 0 720 320"
      className="w-full"
      role="img"
      aria-label="国家和地区访问分布，点击有数据的地区筛选"
    >
      {world.map((c, i) => {
        const n = counts.get(c.code) || 0;
        return (
          <path
            key={`${c.code}-${i}`}
            d={c.path}
            stroke="currentColor"
            strokeOpacity=".25"
            strokeWidth=".5"
            fill={
              n ? `rgba(59,130,246,${0.2 + (0.8 * n) / max})` : "currentColor"
            }
            fillOpacity={n ? 1 : 0.04}
            className={n ? "cursor-pointer hover:stroke-blue-600" : ""}
            onClick={() => {
              if (n) onCountry(c.code);
            }}
            tabIndex={n ? 0 : undefined}
            role={n ? "button" : undefined}
            onKeyDown={(e) => {
              if (n && (e.key === "Enter" || e.key === " ")) {
                e.preventDefault();
                onCountry(c.code);
              }
            }}
            aria-label={`${c.name}：${n} 次浏览`}
          >
            <title>
              {c.name}：{number(n)} 次浏览
            </title>
          </path>
        );
      })}
    </svg>
  );
}
