import type { TimelinePoint } from "@/lib/stock-analysis";
export type { TimelinePoint, AccountSummary, HoldingDistribution, Insight, AnalysisIdea, StockAnalysisReport } from "@/lib/stock-analysis";

export type TrendSeries = {
  key: keyof TimelinePoint;
  label: string;
  color: string;
  axis: "money" | "change" | "percent";
};
