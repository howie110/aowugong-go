import { requestJSON } from "@/lib/request";
export type Metrics = {
  visitors: number;
  visits: number;
  views: number;
  bounce_rate: number;
  duration: number;
};
export type Rank = { name: string; count: number };
export type AnalyticsReport = {
  current: Metrics;
  previous: Metrics;
  from: string;
  to: string;
  geo_enabled: boolean;
  trend: { time: string; visitors: number; views: number }[];
  ranks: Record<string, Rank[]>;
  heat: { day: number; hour: number; count: number }[];
};
export function getAnalytics(
  from: Date,
  to: Date,
  filters: Record<string, string>,
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({
    ...filters,
    from: from.toISOString(),
    to: to.toISOString(),
  });
  return requestJSON<AnalyticsReport>(
    `/api/v1/analytics/report/?${query}`,
    { signal },
    "读取访问统计失败",
  );
}
