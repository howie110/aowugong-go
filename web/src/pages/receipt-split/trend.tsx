import type { ReceiptDay } from "@/lib/receipt-split";
export function ReceiptTrend({ days }: { days: ReceiptDay[] }) {
  const max = Math.max(
    1,
    ...days.flatMap((d) => [d.analysis_users, d.bill_users]),
  );
  const x = (i: number) => 50 + (i * 900) / Math.max(1, days.length - 1);
  const y = (n: number) => 215 - (n / max) * 185;
  return (
    <>
      <svg
        viewBox="0 0 980 260"
        className="w-full"
        role="img"
        aria-label="每日识别用户与生成账单用户趋势"
      >
        {[0, 0.5, 1].map((r) => (
          <g key={r}>
            <line
              x1="50"
              x2="950"
              y1={y(max * r)}
              y2={y(max * r)}
              stroke="currentColor"
              opacity=".1"
            />
            <text
              x="40"
              y={y(max * r) + 4}
              textAnchor="end"
              fontSize="11"
              fill="currentColor"
            >
              {(max * r).toLocaleString("zh-CN", { maximumFractionDigits: 1 })}
            </text>
          </g>
        ))}
        {(["analysis_users", "bill_users"] as const).map((key, j) => (
          <g key={key}>
            <polyline
              points={days.map((d, i) => `${x(i)},${y(d[key])}`).join(" ")}
              stroke={j ? "#14b8a6" : "#3b82f6"}
              strokeWidth="2.5"
              fill="none"
            />
            {days.map((d, i) => (
              <circle
                key={d.date}
                cx={x(i)}
                cy={y(d[key])}
                r={days.length > 90 ? 1 : 3}
                fill={j ? "#14b8a6" : "#3b82f6"}
              >
                <title>
                  {d.date} · {j ? "生成账单用户" : "识别用户"} {d[key]}
                </title>
              </circle>
            ))}
          </g>
        ))}
        {days
          .filter(
            (_, i) =>
              i === 0 ||
              i === days.length - 1 ||
              i === Math.floor(days.length / 2),
          )
          .map((d) => (
            <text
              key={d.date}
              x={x(days.indexOf(d))}
              y="246"
              textAnchor={
                d === days[0]
                  ? "start"
                  : d === days[days.length - 1]
                    ? "end"
                    : "middle"
              }
              fontSize="11"
              fill="currentColor"
            >
              {d.date}
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
