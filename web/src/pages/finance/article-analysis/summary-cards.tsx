import { ChevronDown } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { ArticleAnalysisReport, DistributionItem } from "@/lib/article-analysis";
import { getDistributionBarClass, translate } from "./market-ui";
import { MARKET_DAYS, TARGET_DAYS } from "./page-constants";
import type { AccountStat } from "./page-utils";
import { DEFAULT_ANALYSIS_MODEL, DEFAULT_ANALYSIS_PROMPT, DEFAULT_PROMPT_VERSION } from "./prompt";

export function MarketPanel({ report }: { report: ArticleAnalysisReport | null }) {
  return (
    <Card>
      <CardHeader className="p-4 pb-3 sm:p-5 sm:pb-3">
        <CardTitle>短期市场判断 · {MARKET_DAYS}天</CardTitle>
        <CardDescription className="hidden sm:block">日更文章只按短期口径统计市场氛围和涨跌预测。</CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-2 gap-3 px-4 pb-4 sm:gap-4 sm:px-5 sm:pb-5">
        <DistributionBlock title="市场氛围" items={report?.mood_distribution || []} />
        <DistributionBlock title="涨跌预测" items={report?.prediction_distribution || []} />
      </CardContent>
    </Card>
  );
}

export function MonitoredAccountsCard({ accounts, articleCount }: { accounts: AccountStat[]; articleCount: number }) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Card className="h-full">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 p-4 pb-3 sm:p-5 sm:pb-3">
        <div className="min-w-0 space-y-1.5">
          <CardTitle>监控公众号</CardTitle>
          <CardDescription>近{TARGET_DAYS}天文章 {articleCount} 篇。</CardDescription>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-8 w-8 shrink-0 sm:hidden"
          aria-expanded={isOpen}
          aria-label={isOpen ? "收起监控公众号" : "展开监控公众号"}
          onClick={() => setIsOpen((current) => !current)}
        >
          <ChevronDown className={["h-4 w-4 transition-transform", isOpen ? "rotate-180" : ""].join(" ")} />
        </Button>
      </CardHeader>
      <CardContent className="px-4 pb-4 sm:px-5 sm:pb-5">
        {accounts.length ? (
          <div
            className={[
              "flex flex-wrap gap-2 transition-[max-height] duration-200 sm:max-h-none sm:overflow-visible",
              isOpen ? "max-h-48 overflow-x-hidden overflow-y-auto pr-1" : "max-h-6 overflow-hidden",
            ].join(" ")}
          >
            {accounts.map((account) => (
              <Badge key={account.name} variant="outline" className="max-w-[9rem] shrink-0 gap-1 truncate">
                <span className="truncate">{account.name}</span>
                <span className="text-muted-foreground">{account.count}</span>
              </Badge>
            ))}
          </div>
        ) : (
          <div className="text-sm text-muted-foreground">暂无公众号信息。</div>
        )}
      </CardContent>
    </Card>
  );
}

export function ModelPromptCard({ report }: { report: ArticleAnalysisReport | null }) {
  const [isOpen, setIsOpen] = useState(false);
  const prompt = report?.analysis_prompt?.trim() || DEFAULT_ANALYSIS_PROMPT;
  const modelName = report?.analysis_model?.trim() || DEFAULT_ANALYSIS_MODEL;
  const promptVersion = report?.prompt_version?.trim() || DEFAULT_PROMPT_VERSION;

  return (
    <Card className="relative h-full overflow-visible">
      <CardHeader className="p-4 pb-3 sm:p-5 sm:pb-3">
        <CardTitle>模型和提示词</CardTitle>
        <CardDescription className="truncate">模型 {modelName}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2 px-4 pb-4 sm:px-5 sm:pb-5">
        <div className="rounded-md border px-3 py-2">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">提示词</div>
              <div className="truncate text-xs text-muted-foreground">{promptVersion}</div>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              aria-expanded={isOpen}
              aria-label="展开提示词"
              onClick={() => setIsOpen((current) => !current)}
            >
              <ChevronDown className={["h-4 w-4 transition-transform", isOpen ? "rotate-180" : ""].join(" ")} />
            </Button>
          </div>
        </div>
        {isOpen && prompt ? (
          <div className="absolute left-0 right-0 top-full z-30 mt-2 rounded-md border bg-background p-3 shadow-lg">
            <pre className="max-h-80 whitespace-pre-wrap break-words overflow-y-auto text-xs leading-relaxed text-muted-foreground">
              {prompt}
            </pre>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function DistributionBlock({ title, items }: { title: string; items: DistributionItem[] }) {
  const total = items.reduce((sum, item) => sum + item.count, 0);
  return (
    <div className="space-y-2">
      <div className="text-sm font-medium">{title}</div>
      {items.length ? (
        items.map((item) => {
          const percent = total ? (item.count / total) * 100 : 0;
          return (
            <div key={item.name} className="space-y-1">
              <div className="flex items-center justify-between gap-3 text-xs">
                <span>{translate(item.name)}</span>
                <span className="tabular-nums text-muted-foreground">{item.count}</span>
              </div>
              <div className="h-2 rounded-full bg-muted">
                <div className={`h-2 rounded-full ${getDistributionBarClass(item.name)}`} style={{ width: `${percent}%` }} />
              </div>
            </div>
          );
        })
      ) : (
        <div className="text-xs text-muted-foreground">暂无数据</div>
      )}
    </div>
  );
}
