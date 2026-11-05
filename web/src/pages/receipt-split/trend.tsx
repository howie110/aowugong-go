import { TrendChart } from "@/components/charts/trend-chart";
import type { ReceiptDay } from "@/lib/receipt-split";

/** ReceiptTrend 展示每日识别用户和生成账单用户，并跟随鼠标显示十字定位。 */
export function ReceiptTrend({ days }: { days: ReceiptDay[] }) {
  return (
    <TrendChart
      data={days.map((day) => ({
        label: day.date,
        primary: day.analysis_users,
        secondary: day.bill_users,
      }))}
      primaryLabel="识别用户"
      secondaryLabel="生成账单用户"
      primaryColor="#3b82f6"
      secondaryColor="#14b8a6"
      ariaLabel="每日识别用户与生成账单用户趋势"
      emptyLabel="暂无使用记录"
    />
  );
}
