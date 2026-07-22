import type { RefObject } from "react";

import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
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
  selectedMember,
  onPageChange,
  onSelect,
  onSelectMember,
}: {
  title: string;
  items: TargetSignalStat[];
  totalCount: number;
  currentPage: number;
  totalPages: number;
  pageSize: number;
  tableRef: RefObject<HTMLDivElement>;
  selectedSignal: TargetSignalStat | null;
  selectedMember: string | null;
  onPageChange: (page: number) => void;
  onSelect: (signal: TargetSignalStat) => void;
  onSelectMember: (signal: TargetSignalStat, member: string) => void;
}) {
  const emptyRows = Array.from({ length: Math.max(0, pageSize - items.length) });

  return (
    <Card className="grid min-h-0 min-w-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_auto] overflow-hidden xl:h-full xl:grid-rows-[auto_minmax(0,1fr)_auto]">
      <CardHeader className="p-3 pb-2 sm:p-5 sm:pb-3">
        <CardTitle>{title}</CardTitle>
        <CardDescription>按概念合并，按总数倒序。</CardDescription>
      </CardHeader>
      <CardContent className="min-h-0 overflow-visible px-3 pb-0 pt-0 sm:px-5 xl:overflow-y-auto xl:px-6 xl:[scrollbar-gutter:stable]">
        <div ref={tableRef} className="min-h-0 overflow-visible pb-1 xl:h-full">
          <Table className="table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[45%] px-1 sm:w-[42%] sm:px-2">标的</TableHead>
                <TableHead className="w-[22%] px-1 sm:w-[25%] sm:px-2">信号</TableHead>
                <TableHead className="w-[14%] whitespace-nowrap px-1 sm:w-[15%] sm:px-2 text-right">净数</TableHead>
                <TableHead className="w-[19%] whitespace-nowrap pl-1 pr-8 sm:w-[18%] sm:pl-2 sm:pr-9 text-right">总数</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow
                  key={`${item.name}-${item.type}`}
                  className={isSameSignal(selectedSignal, item) ? "bg-muted/70" : ""}
                >
                  <TableCell colSpan={4} className="p-0">
                    {item.members.length ? (
                      <Accordion type="single" collapsible className="w-full">
                        <AccordionItem value={`${item.type}-${item.name}`} className="relative border-0">
                          <SignalRankSummary item={item} onSelect={onSelect} />
                          <div className="absolute right-0 top-0 flex h-10 items-center">
                            <AccordionTrigger className="h-8 w-8 flex-none justify-center p-0 hover:no-underline">
                              <span className="sr-only">展开 {item.name} 标的群</span>
                            </AccordionTrigger>
                          </div>
                          <AccordionContent
                            aria-label={formatSignalMembers(item.members)}
                            className="border-t px-1 pb-2 pt-2 sm:px-2"
                          >
                            <div className="flex flex-wrap gap-1">
                              {item.members.map((member) => (
                                <Badge
                                  key={`${item.type}-${item.name}-${member}`}
                                  asChild
                                  variant={isSameSignal(selectedSignal, item) && selectedMember === member ? "default" : "outline"}
                                  className="max-w-full cursor-pointer whitespace-normal break-words"
                                >
                                  <button
                                    type="button"
                                    aria-pressed={isSameSignal(selectedSignal, item) && selectedMember === member}
                                    className="gap-1.5"
                                    onClick={() => onSelectMember(item, member)}
                                  >
                                    <span>{member}</span>
                                    <SignalNetCell
                                      value={item.member_net_counts?.[member] ?? 0}
                                      compact
                                      inverted={isSameSignal(selectedSignal, item) && selectedMember === member}
                                    />
                                  </button>
                                </Badge>
                              ))}
                            </div>
                          </AccordionContent>
                        </AccordionItem>
                      </Accordion>
                    ) : (
                      <SignalRankSummary item={item} onSelect={onSelect} />
                    )}
                  </TableCell>
                </TableRow>
              ))}
              {emptyRows.map((_, index) => (
                <TableRow key={`empty-${index}`} data-placeholder-row="true">
                  <TableCell className="px-1 py-2 text-muted-foreground sm:px-2">-</TableCell>
                  <TableCell className="px-1 py-2 text-muted-foreground sm:px-2">-</TableCell>
                  <TableCell className="px-1 py-2 text-right text-muted-foreground sm:px-2">-</TableCell>
                  <TableCell className="px-1 py-2 text-right text-muted-foreground sm:px-2">-</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
      {totalCount ? (
        <div className="px-3 pb-3 sm:px-5 sm:pb-4 xl:px-6">
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

// SignalRankSummary 渲染信号榜单行的四列摘要，并保留独立的筛选点击区域。
// 输入：item 是概念组统计，onSelect 接收用户选择的概念组。
// 输出：返回横跨四列的可点击摘要。
// 副作用：点击时调用 onSelect 筛选文章。
function SignalRankSummary({ item, onSelect }: { item: TargetSignalStat; onSelect: (signal: TargetSignalStat) => void }) {
  // 1. 使用与表头一致的列比例渲染名称、信号、净数和总数。
  return (
    <button
      type="button"
      className="grid min-h-10 w-full grid-cols-[45fr_22fr_14fr_19fr] sm:grid-cols-[42fr_25fr_15fr_18fr] items-center text-left transition-colors hover:bg-muted/50"
      onClick={() => onSelect(item)}
    >
      <span className="min-w-0 truncate px-1 font-medium sm:px-2">{item.name}</span>
      <span className="px-1 sm:px-2">
        <SignalCountCell recommendationCount={item.recommendation_count} riskCount={item.risk_count} />
      </span>
      <span className="px-1 text-right sm:px-2">
        <SignalNetCell value={item.recommendation_count - item.risk_count} />
      </span>
      <span className="whitespace-nowrap pl-1 pr-8 text-right text-sm font-semibold tabular-nums sm:pl-2 sm:pr-9 sm:text-lg">
        {item.count}
      </span>
    </button>
  );
}

function SignalCountCell({ recommendationCount, riskCount }: { recommendationCount: number; riskCount: number }) {
  return (
    <div className="flex items-center gap-0.5 whitespace-nowrap text-sm font-semibold tabular-nums sm:gap-1 sm:text-lg">
      <span className="text-red-700">{recommendationCount}</span>
      <span className="text-muted-foreground">-</span>
      <span className="text-emerald-700">{riskCount}</span>
    </div>
  );
}

// SignalNetCell 使用统一的红涨绿跌规则展示净数，并支持标签内的紧凑样式。
// 输入：value 是推荐数减风险数，compact 控制字号，inverted 适配深色选中标签。
// 输出：返回带符号和语义颜色的净数文本。
// 副作用：无。
function SignalNetCell({ value, compact = false, inverted = false }: { value: number; compact?: boolean; inverted?: boolean }) {
  // 1. 根据数值方向和标签背景选择保持对比度的语义颜色。
  const className = inverted
    ? value > 0
      ? "text-red-300"
      : value < 0
        ? "text-emerald-300"
        : "text-primary-foreground/70"
    : value > 0
      ? "text-red-700"
      : value < 0
        ? "text-emerald-700"
        : "text-neutral-700";

  // 2. 正数补充加号，紧凑模式沿用标签字号。
  const text = value > 0 ? `+${value}` : String(value);
  const sizeClassName = compact ? "text-xs" : "text-sm sm:text-lg";
  return <span className={`flex-none font-semibold tabular-nums ${sizeClassName} ${className}`}>{text}</span>;
}
