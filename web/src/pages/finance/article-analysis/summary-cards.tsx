import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import type { ArticleAnalysisReport, DistributionItem } from "@/lib/article-analysis";
import { getDistributionBarClass, translate } from "./market-ui";
import { MARKET_DAYS } from "./page-constants";

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
              <Progress value={percent} indicatorClassName={getDistributionBarClass(item.name)} />
            </div>
          );
        })
      ) : (
        <div className="text-xs text-muted-foreground">暂无数据</div>
      )}
    </div>
  );
}
