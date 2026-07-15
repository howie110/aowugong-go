import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { getInsightDetail, getInsightValue, maskAccountText } from "./format";
import type { AnalysisIdea, Insight } from "./types";

export function InsightGrid({ insights, isSensitiveMasked }: { insights: Insight[]; isSensitiveMasked: boolean }) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {insights.map((item) => (
        <Card key={item.title}>
          <CardHeader>
            <CardTitle className="text-sm">{item.title}</CardTitle>
            <CardDescription>{getInsightDetail(item, isSensitiveMasked)}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="tabular-nums text-xl font-semibold">{getInsightValue(item, isSensitiveMasked)}</div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

export function IdeasPanel({ ideas, isSensitiveMasked }: { ideas: AnalysisIdea[]; isSensitiveMasked: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>可以继续扩展的分析</CardTitle>
        <CardDescription>这些模块需要更多数据源，先作为后续迭代方向。</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {ideas.map((idea) => (
          <div key={idea.title} className="rounded-md border px-3 py-3">
            <div className="text-sm font-medium">{idea.title}</div>
            <div className="mt-1 text-xs leading-5 text-muted-foreground">{maskAccountText(isSensitiveMasked, idea.description)}</div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
