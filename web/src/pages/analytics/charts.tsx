import { formatBeijingDateTime } from "@/lib/beijing-time";
import type { AnalyticsReport } from "@/lib/analytics";
import world from "./world.json";

export const number = (n: number) =>
  n.toLocaleString("zh-CN", { maximumFractionDigits: 1 });
export const timeLabel = (s: string) =>
  formatBeijingDateTime(s, { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }, s);
export function Trend({ data }: { data: AnalyticsReport["trend"] }) {
  const max = Math.max(1, ...data.map((p) => p.views));
  const point = (v: number, i: number) =>
    `${50 + (i * 900) / Math.max(data.length - 1, 1)},${235 - (v / max) * 205}`;
  return (
    <div className="w-full overflow-hidden">
      <svg
        viewBox="0 0 980 285"
        className="w-full min-h-44"
        role="img"
        aria-label="访客与浏览量趋势，蓝色为浏览量，青色为访客"
      >
        {[0, 0.25, 0.5, 0.75, 1].map((v) => (
          <g key={v}>
            <line
              x1="50"
              x2="950"
              y1={235 - v * 205}
              y2={235 - v * 205}
              stroke="currentColor"
              opacity=".1"
            />
            <text
              x="40"
              y={239 - v * 205}
              textAnchor="end"
              fontSize="11"
              fill="currentColor"
            >
              {number(max * v)}
            </text>
          </g>
        ))}
        {(["views", "visitors"] as const).map((key, j) => (
          <g key={key}>
            <polyline
              points={data.map((p, i) => point(p[key], i)).join(" ")}
              fill="none"
              stroke={j ? "#06b6d4" : "#3b82f6"}
              strokeWidth="2.5"
            />
            {data.map((p, i) => (
              <circle
                key={p.time}
                cx={50 + (i * 900) / Math.max(data.length - 1, 1)}
                cy={235 - (p[key] / max) * 205}
                r={data.length > 80 ? 1 : 3}
                fill={j ? "#06b6d4" : "#3b82f6"}
              >
                <title>
                  {timeLabel(p.time)}：{key === "views" ? "浏览量" : "访客"}{" "}
                  {p[key]}
                </title>
              </circle>
            ))}
          </g>
        ))}
        {data
          .filter(
            (_, i) =>
              i === 0 ||
              i === data.length - 1 ||
              i === Math.floor(data.length / 2),
          )
          .map((p) => (
            <text
              key={p.time}
              x={50 + (data.indexOf(p) * 900) / Math.max(data.length - 1, 1)}
              y="266"
              textAnchor={
                p === data[0]
                  ? "start"
                  : p === data[data.length - 1]
                    ? "end"
                    : "middle"
              }
              fontSize="11"
              fill="currentColor"
            >
              {timeLabel(p.time)}
            </text>
          ))}
      </svg>
      <div className="flex justify-center gap-5 text-xs text-muted-foreground">
        <span className="text-blue-500">● 浏览量</span>
        <span className="text-cyan-500">● 访客</span>
      </div>
    </div>
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
