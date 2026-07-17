import { ArrowDownRight, Sparkles } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  ArticleSource,
  analyzePendingArticles,
  fetchArticleFetchSummary,
  fetchArticleSources,
  runScheduledArticleSync,
} from "@/lib/article-analysis";
import { notify } from "@/lib/notify";

type FetchSummary = {
  title: string;
  description: string;
  metrics: Array<{ label: string; value: string; detail: string }>;
};

export function ArticleFetchPage() {
  const [summary, setSummary] = useState<FetchSummary | null>(null);
  const [sources, setSources] = useState<ArticleSource[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isWorking, setIsWorking] = useState(false);

  const sourceStatus = useMemo(() => {
    const activeCount = sources.filter((source) => source.is_active).length;
    const failedCount = sources.filter((source) => source.last_fetch_status === "error").length;
    return { activeCount, failedCount };
  }, [sources]);

  async function loadData() {
    setIsLoading(true);
    try {
      const [nextSummary, nextSources] = await Promise.all([fetchArticleFetchSummary(), fetchArticleSources()]);
      setSummary(nextSummary);
      setSources(nextSources);
    } catch (error) {
      notify.errorFrom(error, "投资文章抓取数据加载失败", "加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  async function handleSync() {
    setIsWorking(true);
    try {
      const result = await runScheduledArticleSync();
      notify.success("抓取并分析完成", result.message || "生产同步任务执行成功。");
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "抓取文章失败");
    } finally {
      setIsWorking(false);
    }
  }

  async function handleAnalyze() {
    setIsWorking(true);
    try {
      const result = await analyzePendingArticles(10);
      notify.success("分析完成", `成功 ${result.analyzed_count} 篇，跳过 ${result.skipped_count} 篇，失败 ${result.error_count} 篇。`);
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "分析文章失败");
    } finally {
      setIsWorking(false);
    }
  }

  if (isLoading && !summary) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-muted-foreground">正在加载投资文章抓取...</CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="outline" size="sm" className="w-28" onClick={() => void handleSync()} disabled={isWorking}>
          <ArrowDownRight className="h-4 w-4" />
          抓取并分析
        </Button>
        <Button type="button" size="sm" className="w-28" onClick={() => void handleAnalyze()} disabled={isWorking}>
          <Sparkles className="h-4 w-4" />
          分析未分析
        </Button>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {(summary?.metrics || []).map((item) => (
          <Card key={item.label}>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm text-muted-foreground">{item.label}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-semibold">{item.value}</div>
              <p className="mt-1 text-xs text-muted-foreground">{item.detail}</p>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <div className="flex flex-col justify-between gap-2 md:flex-row md:items-start">
            <div>
              <CardTitle>信息源</CardTitle>
              <CardDescription>来源只用于归属和抓取，不参与推荐/风险统计口径。</CardDescription>
            </div>
            <div className="flex gap-2">
              <Badge variant="outline">启用 {sourceStatus.activeCount}</Badge>
              {sourceStatus.failedCount ? <Badge variant="danger">失败 {sourceStatus.failedCount}</Badge> : null}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>来源</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>最后抓取</TableHead>
                <TableHead>状态</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sources.map((source) => (
                <TableRow key={source.id}>
                  <TableCell>
                    <div className="font-medium">{source.source_name}</div>
                    <div className="text-xs text-muted-foreground">{source.last_fetch_message || source.description}</div>
                  </TableCell>
                  <TableCell>{source.source_type}</TableCell>
                  <TableCell className="text-muted-foreground">{formatDate(source.last_fetch_at)}</TableCell>
                  <TableCell>
                    <Badge variant={source.is_active ? (source.last_fetch_status === "error" ? "danger" : "success") : "secondary"}>
                      {source.last_fetch_status || (source.is_active ? "ready" : "disabled")}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}

function formatDate(value?: string | null) {
  if (!value) {
    return "暂无";
  }
  return new Date(value).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}
