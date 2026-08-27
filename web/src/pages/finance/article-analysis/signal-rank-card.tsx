import type { RefObject } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";

import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { TargetSignalStat } from "@/lib/article-analysis";
import { ArticlePagination } from "./article-pagination";
import {
  formatSignalMembers,
  isSameSignal,
  type SignalSortDirection,
  type SignalSortField,
} from "./page-utils";
import { SignalNetTrend } from "./signal-net-trend";

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
  sortField,
  sortDirection,
  onPageChange,
  onSort,
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
  sortField: SignalSortField;
  sortDirection: SignalSortDirection;
  onPageChange: (page: number) => void;
  onSort: (field: SignalSortField) => void;
  onSelect: (signal: TargetSignalStat) => void;
  onSelectMember: (signal: TargetSignalStat, member: string) => void;
}) {
  const emptyRows = Array.from({ length: Math.max(0, pageSize - items.length) });

  return (
    <Card className="grid min-h-0 min-w-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_auto] overflow-hidden">
      <CardHeader className="p-3 pb-2 sm:p-5 sm:pb-3">
        <CardTitle>{title}</CardTitle>
        <CardDescription>按概念合并，可按净数或总数排序。</CardDescription>
      </CardHeader>
      <CardContent className="min-h-0 overflow-x-auto px-3 pb-0 pt-0 sm:px-5 xl:px-6">
        <div ref={tableRef} className="min-h-0 min-w-[64rem] pb-1">
          <Table className="table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[18%] px-2">标的</TableHead>
                <TableHead className="w-[11%] px-2">信号</TableHead>
                <TableHead className="w-[7%] whitespace-nowrap px-2">
                  <SignalSortHeader label="净数" field="net" activeField={sortField} direction={sortDirection} onSort={onSort} />
                </TableHead>
                <TableHead className="w-[6%] whitespace-nowrap px-2">
                  <SignalSortHeader label="总数" field="total" activeField={sortField} direction={sortDirection} onSort={onSort} />
                </TableHead>
                <TableHead className="w-[58%] px-2 pr-10">净数变化图</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow
                  key={`${item.name}-${item.type}`}
                  className={isSameSignal(selectedSignal, item) ? "bg-muted/70" : ""}
                >
                  <TableCell colSpan={5} className="p-0">
                    {item.members.length ? (
                      <Accordion type="single" collapsible className="w-full">
                        <AccordionItem value={`${item.type}-${item.name}`} className="relative border-0">
                          <SignalRankSummary item={item} onSelect={onSelect} />
                          <div className="absolute right-1 top-14 z-10 flex -translate-y-1/2 items-center">
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
                  <TableCell className="h-28 px-2 text-muted-foreground">-</TableCell>
                  <TableCell className="h-28 px-2 text-muted-foreground">-</TableCell>
                  <TableCell className="h-28 px-2 text-right text-muted-foreground">-</TableCell>
                  <TableCell className="h-28 px-2 text-right text-muted-foreground">-</TableCell>
                  <TableCell className="h-28 px-2 text-muted-foreground">-</TableCell>
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

function SignalSortHeader({
  label,
  field,
  activeField,
  direction,
  onSort,
}: {
  label: string;
  field: SignalSortField;
  activeField: SignalSortField;
  direction: SignalSortDirection;
  onSort: (field: SignalSortField) => void;
}) {
  const isActive = field === activeField;
  const Icon = !isActive ? ArrowUpDown : direction === "asc" ? ArrowUp : ArrowDown;
  return (
    <div className="flex items-center gap-0.5">
      <span>{label}</span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-7 w-7"
        aria-label={`${label}${isActive ? (direction === "asc" ? "升序" : "降序") : "排序"}`}
        onClick={() => onSort(field)}
      >
        <Icon className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

// SignalRankSummary 渲染信号榜单行的五列摘要，并保留独立的筛选点击区域。
// 输入：item 是概念组统计，onSelect 接收用户选择的概念组。
// 输出：返回横跨五列的可点击摘要。
// 副作用：点击时调用 onSelect 筛选文章。
function SignalRankSummary({ item, onSelect }: { item: TargetSignalStat; onSelect: (signal: TargetSignalStat) => void }) {
  // 1. 使用与表头一致的列比例渲染名称、信号、净数、总数和每日趋势。
  return (
    <button
      type="button"
      className="grid min-h-28 w-full grid-cols-[18fr_11fr_7fr_6fr_58fr] items-center text-left transition-colors hover:bg-muted/50"
      onClick={() => onSelect(item)}
    >
      <span className="min-w-0 truncate px-2 font-medium">{item.name}</span>
      <span className="px-2">
        <SignalCountCell recommendationCount={item.recommendation_count} riskCount={item.risk_count} />
      </span>
      <span className="px-2">
        <SignalNetCell value={item.recommendation_count - item.risk_count} />
      </span>
      <span className="whitespace-nowrap px-2 text-lg font-semibold tabular-nums">
        {item.count}
      </span>
      <span className="min-w-0 px-2 pr-10">
        <SignalNetTrend points={item.net_history || []} />
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
