export type TimelinePoint = {
  snapshot_date: string;
  total_asset: string | number;
  market_value: string | number;
  available_cash: string | number;
  other_amount: string | number;
  position_percent: string | number;
  daily_change: string | number;
  cumulative_change: string | number;
  account_count: number;
};

export type AccountSummary = {
  account_suffix: string;
  account_alias: string;
  broker_name: string;
  snapshot_date: string;
  total_asset: string | number;
  market_value: string | number;
  available_cash: string | number;
  other_amount: string | number;
  position_percent: string | number;
  daily_change: string | number;
  cumulative_change: string | number;
};

export type HoldingDistribution = {
  security_name: string;
  market_value: string | number;
  quantity?: string | number | null;
  weight_percent: string | number;
  account_count: number;
  accounts: string;
};

export type Insight = {
  title: string;
  value: string;
  detail: string;
};

export type AnalysisIdea = {
  title: string;
  description: string;
};

export type StockAnalysisReport = {
  latest?: TimelinePoint | null;
  first?: TimelinePoint | null;
  previous?: TimelinePoint | null;
  changes: {
    total_asset_change: string | number;
    market_value_change: string | number;
    available_cash_change: string | number;
    daily_total_asset_change: string | number;
  };
  timeline: TimelinePoint[];
  accounts: AccountSummary[];
  holdings: HoldingDistribution[];
  insights: Insight[];
  ideas: AnalysisIdea[];
  snapshot_count: number;
  date_count: number;
};

export type TrendSeries = {
  key: keyof TimelinePoint;
  label: string;
  color: string;
  axis: "money" | "percent";
};
