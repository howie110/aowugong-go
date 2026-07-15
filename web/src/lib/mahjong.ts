import { authorizedFetch } from "@/lib/auth";

export type MahjongRecord = {
  id: number;
  played_date: string;
  result_amount: string;
  source_filename?: string | null;
  created_by?: string | null;
  created_at?: string | null;
  updated_at?: string | null;
};

export type MahjongExtremeDay = {
  played_date?: string | null;
  result_amount: string;
};

export type MahjongSummary = {
  total_games: number;
  win_games: number;
  loss_games: number;
  draw_games: number;
  win_rate: string;
  total_result: string;
  average_result: string;
  table_fee: string;
  adjusted_average_result: string;
  first_date?: string | null;
  latest_date?: string | null;
  span_days: number;
  span_years: string;
  best_day: MahjongExtremeDay;
  worst_day: MahjongExtremeDay;
  current_streak_type: string;
  current_streak_count: number;
  longest_win_streak: number;
  longest_loss_streak: number;
};

export type MahjongTimelinePoint = {
  sequence: number;
  played_date: string;
  result_amount: string;
  cumulative_result: string;
  running_average: string;
};

export type MahjongPeriodSummary = {
  period: string;
  game_count: number;
  win_count: number;
  loss_count: number;
  win_rate: string;
  total_result: string;
  average_result: string;
};

export type MahjongGap = {
  start_date?: string | null;
  end_date?: string | null;
  days: number;
};

export type MahjongWeekdayFrequency = {
  weekday: number;
  label: string;
  game_count: number;
  weight_percent: string;
};

export type MahjongFrequencyStats = {
  active_months: number;
  average_games_per_month: string;
  average_days_between_games: string;
  recent_90_day_games: number;
  recent_365_day_games: number;
  most_active_month?: string | null;
  most_active_month_games: number;
  favorite_weekday?: string | null;
  favorite_weekday_games: number;
  longest_gap: MahjongGap;
  weekday_distribution: MahjongWeekdayFrequency[];
};

export type MahjongReport = {
  summary: MahjongSummary;
  frequency: MahjongFrequencyStats;
  timeline: MahjongTimelinePoint[];
  monthly: MahjongPeriodSummary[];
  yearly: MahjongPeriodSummary[];
  recent_records: MahjongRecord[];
  record_count: number;
};

export type MahjongRecordWriteResponse = {
  status: string;
  record: MahjongRecord;
};

export async function fetchMahjongReport(tableFee = "9") {
  const response = await authorizedFetch(`/api/v1/mahjong/report?table_fee=${encodeURIComponent(tableFee)}`);
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "读取麻将战绩失败");
  }
  return (await response.json()) as MahjongReport;
}

export async function saveMahjongRecord(playedDate: string, resultAmount: string) {
  const response = await authorizedFetch("/api/v1/mahjong/records/save", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      played_date: playedDate,
      result_amount: resultAmount,
    }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "保存麻将战绩失败");
  }
  return (await response.json()) as MahjongRecordWriteResponse;
}
