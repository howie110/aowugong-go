import { useEffect, useState } from "react";
import { AlertCircle } from "lucide-react";

import { Card, CardContent } from "@/components/ui/card";
import { clearToken, getToken } from "@/lib/auth";
import { AccountGrid } from "./stock-analysis/account-cards";
import { EmptyAnalysis } from "./stock-analysis/empty-analysis";
import { HoldingDistributionCard } from "./stock-analysis/holding-distribution-card";
import { IdeasPanel, InsightGrid } from "./stock-analysis/insight-panels";
import { RecentTimelineTable } from "./stock-analysis/recent-timeline-table";
import { SummaryGrid } from "./stock-analysis/summary-grid";
import { CombinedTrendCard } from "./stock-analysis/trend-chart";
import type { StockAnalysisReport } from "./stock-analysis/types";

async function authorizedFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    clearToken();
    window.location.href = "/login";
  }
  return response;
}

export function StockAnalysisPage({ isSensitiveMasked }: { isSensitiveMasked: boolean }) {
  const [report, setReport] = useState<StockAnalysisReport | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    void loadReport();
  }, []);

  async function loadReport() {
    setIsLoading(true);
    setMessage(null);
    try {
      const response = await authorizedFetch("/api/v1/finance/stock-analysis/report?limit=500");
      if (!response.ok) {
        throw new Error("读取股票仓位分析失败");
      }
      setReport((await response.json()) as StockAnalysisReport);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "读取股票仓位分析失败");
    } finally {
      setIsLoading(false);
    }
  }

  if (message) {
    return (
      <div className="rounded-md border border-destructive/40 bg-destructive/5 px-3 py-3 text-sm text-destructive">
        <div className="flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {message}
        </div>
      </div>
    );
  }

  if (isLoading || !report) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-muted-foreground">正在生成股票仓位分析...</CardContent>
      </Card>
    );
  }

  if (!report.timeline.length || !report.latest) {
    return <EmptyAnalysis />;
  }

  return (
    <div className="space-y-4">
      <SummaryGrid report={report} isSensitiveMasked={isSensitiveMasked} />
      <CombinedTrendCard data={report.timeline} isSensitiveMasked={isSensitiveMasked} />

      <div className="grid gap-4 xl:grid-cols-[1.15fr_0.85fr]">
        <HoldingDistributionCard holdings={report.holdings} latest={report.latest} isSensitiveMasked={isSensitiveMasked} />
        <InsightGrid insights={report.insights} isSensitiveMasked={isSensitiveMasked} />
      </div>

      <AccountGrid accounts={report.accounts} isSensitiveMasked={isSensitiveMasked} />

      <div className="grid gap-4 xl:grid-cols-[1fr_0.85fr]">
        <RecentTimelineTable data={report.timeline} isSensitiveMasked={isSensitiveMasked} />
        <IdeasPanel ideas={report.ideas} isSensitiveMasked={isSensitiveMasked} />
      </div>
    </div>
  );
}
