import type { RefObject } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { TargetSignalStat } from "@/lib/article-analysis";
import { ArticlePagination } from "./article-pagination";
import { formatSignalMembers, isSameSignal } from "./page-utils";

export function SignalRankCard({
  title,
  items,
  totalCount,
  currentPage,
  totalPages,
  pageSize,
  tableRef,
  selectedSignal,
  onPageChange,
  onSelect,
}: {
  title: string;
  items: TargetSignalStat[];
  totalCount: number;
  currentPage: number;
  totalPages: number;
  pageSize: number;
  tableRef: RefObject<HTMLDivElement>;
  selectedSignal: TargetSignalStat | null;
  onPageChange: (page: number) => void;
  onSelect: (signal: TargetSignalStat | null) => void;
}) {
  const emptyRows = Array.from({ length: Math.max(0, pageSize - items.length) });

  return (
    <Card className="grid min-h-0 min-w-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_auto] overflow-hidden xl:h-full xl:grid-rows-[auto_minmax(0,1fr)_auto]">
      <CardHeader className="p-4 pb-3 sm:p-5 sm:pb-3">
        <CardTitle>{title}</CardTitle>
        <CardDescription>按概念合并，按总数倒序。</CardDescription>
      </CardHeader>
      <CardContent className="min-h-0 overflow-visible px-4 pb-0 pt-0 sm:px-5 xl:overflow-y-auto xl:px-6">
        <div ref={tableRef} className="min-h-0 overflow-visible pb-1 xl:h-full">
          <Table className="table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[50%] px-2">标的</TableHead>
                <TableHead className="w-[22%] px-2">信号</TableHead>
                <TableHead className="w-[14%] px-2 text-right">净数</TableHead>
                <TableHead className="w-[14%] px-2 text-right">总数</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow
                  key={`${item.name}-${item.type}`}
                  onClick={() => onSelect(isSameSignal(selectedSignal, item) ? null : item)}
                  className={["cursor-pointer", isSameSignal(selectedSignal, item) ? "bg-muted/70" : ""].join(" ")}
                >
                  <TableCell className="px-2 py-2 align-top">
                    <div className="font-medium">{item.name}</div>
                    {item.members.length ? (
                      <div
                        aria-label={formatSignalMembers(item.members)}
                        className="mt-1 flex flex-wrap gap-x-1 text-xs font-normal leading-5 text-muted-foreground"
                      >
                        {item.members.map((name, index) => (
                          <span key={name} className="whitespace-nowrap">
                            {index ? `· ${name}` : name}
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="px-2 py-2">
                    <SignalCountCell recommendationCount={item.recommendation_count} riskCount={item.risk_count} />
                  </TableCell>
                  <TableCell className="px-2 py-2 text-right">
                    <SignalNetCell value={item.recommendation_count - item.risk_count} />
                  </TableCell>
                  <TableCell className="px-2 py-2 text-right text-lg font-semibold tabular-nums">{item.count}</TableCell>
                </TableRow>
              ))}
              {emptyRows.map((_, index) => (
                <TableRow key={`empty-${index}`} data-placeholder-row="true">
                  <TableCell className="px-2 py-2 text-muted-foreground">-</TableCell>
                  <TableCell className="px-2 py-2 text-muted-foreground">-</TableCell>
                  <TableCell className="px-2 py-2 text-right text-muted-foreground">-</TableCell>
                  <TableCell className="px-2 py-2 text-right text-muted-foreground">-</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
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

function SignalCountCell({ recommendationCount, riskCount }: { recommendationCount: number; riskCount: number }) {
  return (
    <div className="flex items-center gap-1 text-lg font-semibold tabular-nums">
      <span className="text-red-700">{recommendationCount}</span>
      <span className="text-muted-foreground">-</span>
      <span className="text-emerald-700">{riskCount}</span>
    </div>
  );
}

function SignalNetCell({ value }: { value: number }) {
  const className = value > 0 ? "text-red-700" : value < 0 ? "text-emerald-700" : "text-neutral-700";
  const text = value > 0 ? `+${value}` : String(value);
  return <span className={`text-lg font-semibold tabular-nums ${className}`}>{text}</span>;
}
