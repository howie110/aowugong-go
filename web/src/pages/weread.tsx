import { useEffect, useState } from "react";
import { ArrowUpRight, BookOpen, CalendarDays, Clock, FileText, Flame } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type {
  WeReadHeatmap,
  WeReadHeatmapDay,
  WeReadMetric,
  WeReadProgressBook,
  WeReadSummaryData,
} from "@/lib/weread";
import { fetchWeReadHeatmap, fetchWeReadProgress, fetchWeReadSummary } from "@/lib/weread";

const metricIcons = [Clock, CalendarDays, FileText, BookOpen];
const heatLevelClasses = [
  "bg-muted",
  "bg-emerald-100",
  "bg-emerald-300",
  "bg-emerald-500",
  "bg-emerald-700",
];

export function WeReadPage() {
  const [summaryData, setSummaryData] = useState<WeReadSummaryData | null>(null);
  const [progressBooks, setProgressBooks] = useState<WeReadProgressBook[]>([]);
  const [heatmap, setHeatmap] = useState<WeReadHeatmap | null>(null);
  const [summaryMessage, setSummaryMessage] = useState<string | null>(null);
  const [progressMessage, setProgressMessage] = useState<string | null>(null);
  const [heatmapMessage, setHeatmapMessage] = useState<string | null>(null);
  const [isSummaryLoading, setIsSummaryLoading] = useState(true);
  const [isProgressLoading, setIsProgressLoading] = useState(true);
  const [isHeatmapLoading, setIsHeatmapLoading] = useState(false);

  useEffect(() => {
    let isCancelled = false;

    async function loadSummary() {
      setIsSummaryLoading(true);
      setSummaryMessage(null);
      try {
        const data = await fetchWeReadSummary();
        if (!isCancelled) {
          setSummaryData(data);
        }
      } catch (error) {
        if (!isCancelled) {
          setSummaryMessage(error instanceof Error ? error.message : "获取微信读书指标失败");
        }
      } finally {
        if (!isCancelled) {
          setIsSummaryLoading(false);
        }
      }
    }

    async function loadProgress() {
      setIsProgressLoading(true);
      setProgressMessage(null);
      try {
        const data = await fetchWeReadProgress();
        if (!isCancelled) {
          setProgressBooks(data.progress_books);
        }
      } catch (error) {
        if (!isCancelled) {
          setProgressMessage(error instanceof Error ? error.message : "获取微信读书进度失败");
        }
      } finally {
        if (!isCancelled) {
          setIsProgressLoading(false);
        }
      }
    }

    async function loadHeatmap() {
      setIsHeatmapLoading(true);
      setHeatmapMessage(null);
      try {
        const data = await fetchWeReadHeatmap();
        if (!isCancelled) {
          setHeatmap(data.heatmap);
        }
      } catch (error) {
        if (!isCancelled) {
          setHeatmapMessage(error instanceof Error ? error.message : "获取微信读书热力图失败");
        }
      } finally {
        if (!isCancelled) {
          setIsHeatmapLoading(false);
        }
      }
    }

    loadSummary();
    loadProgress();
    const heatmapTimer = window.setTimeout(loadHeatmap, 300);

    return () => {
      isCancelled = true;
      window.clearTimeout(heatmapTimer);
    };
  }, []);

  return (
    <div className="space-y-4">
      <MetricGrid metrics={summaryData?.metrics || []} isLoading={isSummaryLoading} message={summaryMessage} />

      <HeatmapCard heatmap={heatmap} isLoading={isHeatmapLoading} message={heatmapMessage} />

      <ReadingProgressCard books={progressBooks.slice(0, 6)} isLoading={isProgressLoading} message={progressMessage} />

      {summaryData ? <div className="text-xs text-muted-foreground">更新时间：{summaryData.summary.updated_at}</div> : null}
    </div>
  );
}

function MetricGrid({
  metrics,
  isLoading,
  message,
}: {
  metrics: WeReadMetric[];
  isLoading: boolean;
  message: string | null;
}) {
  if (message) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading || !metrics.length) {
    return (
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {["累计阅读", "阅读天数", "笔记总数", "最近阅读"].map((label, index) => {
          const Icon = metricIcons[index] || BookOpen;
          return (
            <Card key={label}>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
                <Icon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-8 w-24" />
              </CardContent>
            </Card>
          );
        })}
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {metrics.map((metric, index) => {
        const Icon = metricIcons[index] || BookOpen;
        return (
          <Card key={metric.label}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground">{metric.label}</CardTitle>
              <Icon className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="break-words text-2xl font-semibold tabular-nums">{metric.value}</div>
              <p className="mt-1 text-xs text-muted-foreground">{metric.detail}</p>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

function HeatmapCard({
  heatmap,
  isLoading,
  message,
}: {
  heatmap: WeReadHeatmap | null;
  isLoading: boolean;
  message: string | null;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2">
            <Flame className="h-4 w-4" />
            年度阅读热力图
          </CardTitle>
          <CardDescription>
            {heatmap ? `过去一年 ${heatmap.active_days} 天有阅读，共 ${heatmap.total_text}` : "热力图数据较慢，会在这里单独加载。"}
          </CardDescription>
        </div>
        <HeatmapLegend />
      </CardHeader>
      <CardContent>
        {message ? <Alert variant="destructive"><AlertDescription>{message}</AlertDescription></Alert> : null}
        {isLoading && !heatmap ? <Skeleton className="h-24 w-full" /> : null}
        {!isLoading && !message && !heatmap ? <CompactEmpty title="暂无阅读热力图" description="产生阅读记录后会显示近一年的分布。" /> : null}
        {heatmap ? <HeatmapGrid days={heatmap.days} /> : null}
      </CardContent>
    </Card>
  );
}

function HeatmapGrid({ days }: { days: WeReadHeatmapDay[] }) {
  return (
        <div className="overflow-x-auto pb-2">
          <div
            className="grid grid-flow-col grid-rows-7 gap-1"
            style={{ gridAutoColumns: "0.75rem" }}
          >
            {days.map((day) => (
              <div
                key={day.date}
                className={cn("h-3 w-3 rounded-[2px]", heatLevelClasses[day.level] || heatLevelClasses[0])}
                title={`${day.date} ${day.minutes} 分钟`}
              />
            ))}
          </div>
        </div>
  );
}

function HeatmapLegend() {
  const labels = ["[0,0]", "(0,0.5]", "(0.5,2]", "(2,5]", "(5,+∞]"];

  return (
    <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {labels.map((label, level) => (
        <span key={label} className="inline-flex items-center gap-1">
          <span className={cn("h-3 w-3 rounded-[2px]", heatLevelClasses[level])} />
          <span className="tabular-nums">{label}</span>
        </span>
      ))}
    </div>
  );
}

function ReadingProgressCard({
  books,
  isLoading,
  message,
}: {
  books: WeReadProgressBook[];
  isLoading: boolean;
  message: string | null;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>最近阅读与进度</CardTitle>
        <CardDescription>按最近阅读时间排序，仅展示最近 6 本书。</CardDescription>
      </CardHeader>
      <CardContent>
        {message ? (
          <Alert variant="destructive"><AlertDescription>{message}</AlertDescription></Alert>
        ) : isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : books.length === 0 ? (
          <CompactEmpty title="暂无最近阅读记录" description="最近阅读的书籍会显示在这里。" />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>书籍</TableHead>
                <TableHead>进度</TableHead>
                <TableHead>阅读时长</TableHead>
                <TableHead className="text-right">最近阅读</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {books.map((book) => (
                <TableRow key={book.book_id}>
                  <TableCell>
                    <div className="max-w-[16rem]">
                      <div className="truncate text-sm font-medium">{book.title}</div>
                      <div className="mt-1 truncate text-xs text-muted-foreground">{book.author}</div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="w-28">
                      <Progress value={clampPercent(book.progress)} />
                      <div className="mt-1 text-xs text-muted-foreground">{clampPercent(book.progress)}%</div>
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{book.reading_time_text}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Badge variant={book.finish_reading ? "success" : "outline"}>
                        {book.finish_reading ? "已读完" : book.read_date}
                      </Badge>
                      <OpenBookLink url={book.open_url} />
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function OpenBookLink({ url }: { url?: string }) {
  if (!url) {
    return null;
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button asChild variant="outline" size="icon" className="h-7 w-7">
          <a href={url} aria-label="打开微信读书">
            <ArrowUpRight className="h-3.5 w-3.5" />
          </a>
        </Button>
      </TooltipTrigger>
      <TooltipContent>打开微信读书</TooltipContent>
    </Tooltip>
  );
}

/** CompactEmpty 展示微信读书模块的紧凑空状态，无副作用。 */
function CompactEmpty({ title, description }: { title: string; description: string }) {
  // 1. 使用统一 Empty 结构展示标题和补充说明。
  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function clampPercent(value: number) {
  return Math.min(Math.max(Math.round(value), 0), 100);
}
