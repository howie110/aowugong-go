import type { RefObject } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { ArticleItem, TargetSignalStat } from "@/lib/article-analysis";
import { ArticlePagination } from "./article-pagination";
import { MarketMoodBadge, MarketPredictionBadge } from "./market-ui";
import { TARGET_DAYS } from "./page-constants";
import { formatShortDate, getSignalToneClass } from "./page-utils";

/** 渲染带分页的文章列表卡片。 */
export function ArticlesCard({
  articles,
  totalCount,
  currentPage,
  totalPages,
  pageSize,
  tableRef,
  selectedArticleId,
  selectedSignal,
  onClearSignal,
  onPageChange,
  onSelect,
}: {
  articles: ArticleItem[];
  totalCount: number;
  currentPage: number;
  totalPages: number;
  pageSize: number;
  tableRef: RefObject<HTMLDivElement>;
  selectedArticleId?: number;
  selectedSignal: TargetSignalStat | null;
  onClearSignal: () => void;
  onPageChange: (page: number) => void;
  onSelect: (article: ArticleItem) => void;
}) {
  return (
    <Card className="grid min-h-0 min-w-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_auto] overflow-hidden xl:h-full xl:grid-rows-[auto_minmax(0,1fr)_auto]">
      <CardHeader className="flex flex-col gap-2 p-4 pb-3 sm:p-5 sm:pb-3 md:flex-row md:items-center md:justify-between">
        <div>
          <CardTitle>文章列表</CardTitle>
          <CardDescription>
            {selectedSignal ? `标的 · ${selectedSignal.name}` : `最近 ${TARGET_DAYS} 天，点击查看分析结果。`}
          </CardDescription>
        </div>
        {selectedSignal ? (
          <Button type="button" variant="outline" size="sm" onClick={onClearSignal}>
            全部文章
          </Button>
        ) : null}
      </CardHeader>
      <CardContent className="min-h-0 overflow-visible px-4 pb-0 pt-0 sm:px-5 xl:overflow-hidden xl:px-6">
        {articles.length ? (
          <>
            <div className="divide-y border-y lg:hidden">
              {articles.map((article) => (
                <MobileArticleRow
                  key={article.id}
                  article={article}
                  isSelected={selectedArticleId === article.id}
                  onSelect={onSelect}
                />
              ))}
            </div>
            <div ref={tableRef} className="hidden min-h-0 overflow-hidden pb-1 pr-1 lg:block xl:h-full">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[26%]">文章</TableHead>
                    <TableHead className="w-[8%]">市场氛围</TableHead>
                    <TableHead className="w-[8%]">涨跌预测</TableHead>
                    <TableHead className="w-[29%]">推荐标的</TableHead>
                    <TableHead className="w-[29%]">风险标的</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {articles.map((article) => (
                    <TableRow
                      key={article.id}
                      onClick={() => onSelect(article)}
                      className={["cursor-pointer", selectedArticleId === article.id ? "bg-muted/70" : ""].join(" ")}
                    >
                      <TableCell className="max-w-[12rem] py-2">
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {formatShortDate(article.published_at || article.created_at)}
                          </span>
                          <span className="truncate text-sm font-medium">{article.title}</span>
                        </div>
                      </TableCell>
                      <TableCell className="py-2">
                        <MarketMoodBadge value={article.market_mood} />
                      </TableCell>
                      <TableCell className="py-2">
                        <MarketPredictionBadge value={article.market_prediction} />
                      </TableCell>
                      <TableCell className="py-2">
                        <SignalNameList names={article.recommendation_names} tone="recommend" />
                      </TableCell>
                      <TableCell className="py-2">
                        <SignalNameList names={article.risk_names} tone="risk" />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        ) : (
          <div ref={tableRef} className="rounded-md border px-3 py-6 text-sm text-muted-foreground">
            暂无文章。
          </div>
        )}
      </CardContent>
      {totalCount ? (
        <div className="px-4 pb-4 sm:px-5 xl:px-6">
          <ArticlePagination
            currentPage={currentPage}
            totalPages={totalPages}
            totalCount={totalCount}
            pageSize={pageSize}
            onPageChange={onPageChange}
          />
        </div>
      ) : null}
    </Card>
  );
}

/** 渲染手机端文章行，左侧集中日期与判断，右侧完整展示标的。 */
function MobileArticleRow({
  article,
  isSelected,
  onSelect,
}: {
  article: ArticleItem;
  isSelected: boolean;
  onSelect: (article: ArticleItem) => void;
}) {
  return (
    <button
      type="button"
      data-mobile-article-row
      className={[
        "block w-full max-w-full px-1 py-3 text-left transition-colors hover:bg-muted/50",
        isSelected ? "bg-muted/70" : "",
      ].join(" ")}
      onClick={() => onSelect(article)}
    >
      <div className="grid min-w-0 grid-cols-[3.75rem_minmax(0,1fr)] gap-x-2">
        <div className="flex min-w-0 flex-col items-center gap-2">
          <span className="w-full text-center text-xs tabular-nums text-muted-foreground">
            {formatShortDate(article.published_at || article.created_at)}
          </span>
          <div className="flex w-full items-center justify-center gap-1">
            <MarketMoodBadge value={article.market_mood} />
            <MarketPredictionBadge value={article.market_prediction} />
          </div>
        </div>
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">{article.title}</div>
          <div className="mt-2 space-y-1.5">
            <MobileSignalRow label="荐" names={article.recommendation_names} tone="recommend" />
            <MobileSignalRow label="险" names={article.risk_names} tone="risk" />
          </div>
        </div>
      </div>
    </button>
  );
}

/** 渲染手机端一类标的，并允许全部标的自动换行。 */
function MobileSignalRow({
  label,
  names,
  tone,
}: {
  label: string;
  names: string[];
  tone: "recommend" | "risk";
}) {
  const labelClassName = tone === "recommend" ? "text-red-700" : "text-emerald-700";

  return (
    <div className="grid min-w-0 grid-cols-[1rem_minmax(0,1fr)] items-start gap-x-1.5">
      <span className={`text-xs font-semibold leading-5 ${labelClassName}`}>{label}</span>
      {names.length ? (
        <div className="flex min-w-0 flex-wrap items-center gap-1">
          {names.map((name) => (
            <Badge key={name} className={`min-w-0 max-w-full whitespace-normal break-words ${getSignalToneClass(tone)}`}>
              {name}
            </Badge>
          ))}
        </div>
      ) : (
        <span className="text-xs leading-5 text-muted-foreground">-</span>
      )}
    </div>
  );
}

/** 渲染桌面端文章表格中的标的列表。 */
function SignalNameList({ names, tone }: { names: string[]; tone: "recommend" | "risk" }) {
  if (!names.length) {
    return <span className="text-xs text-muted-foreground">-</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {names.map((name) => (
        <Badge key={name} className={`max-w-[7rem] truncate ${getSignalToneClass(tone)}`}>
          {name}
        </Badge>
      ))}
    </div>
  );
}
