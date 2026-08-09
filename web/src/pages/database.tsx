import { Database, Download, HardDrive, Search, TableProperties } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Pagination, PaginationContent, PaginationItem, PaginationNext, PaginationPrevious } from "@/components/ui/pagination";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  type DatabaseColumn,
  type DatabaseRowsPage,
  type DatabaseSummary,
  downloadDatabaseTable,
  fetchDatabaseRows,
  fetchDatabaseSummary,
} from "@/lib/database";
import { notify } from "@/lib/notify";

const pageSizeOptions = [25, 50, 100, 200];

/** DatabasePage 展示管理员只读数据库概况、表数据、搜索和脱敏导出。 */
export function DatabasePage() {
  const [summary, setSummary] = useState<DatabaseSummary | null>(null);
  const [selectedTable, setSelectedTable] = useState("");
  const [rowsPage, setRowsPage] = useState<DatabaseRowsPage | null>(null);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [isSummaryLoading, setIsSummaryLoading] = useState(true);
  const [isRowsLoading, setIsRowsLoading] = useState(false);
  const [isExporting, setIsExporting] = useState(false);

  useEffect(() => {
    // 1. 页面进入时读取表清单并默认选中首张有数据的表。
    fetchDatabaseSummary()
      .then((result) => {
        setSummary(result);
        setSelectedTable((current) => (
          current || result.tables.find((table) => table.row_count > 0)?.name || result.tables[0]?.name || ""
        ));
      })
      .catch((error) => notify.errorFrom(error, "读取数据库概况失败", "加载失败"))
      .finally(() => setIsSummaryLoading(false));
  }, []);

  useEffect(() => {
    // 1. 输入停止短暂时间后再执行服务器搜索。
    const timer = window.setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    // 1. 表、页码或筛选变化时读取当前页，并忽略已经过期的响应。
    if (!selectedTable) {
      setRowsPage(null);
      return;
    }
    let cancelled = false;
    setIsRowsLoading(true);
    fetchDatabaseRows(selectedTable, page, pageSize, search)
      .then((result) => {
        if (!cancelled) {
          setRowsPage(result);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          notify.errorFrom(error, "读取数据库表失败", "加载失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsRowsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [page, pageSize, search, selectedTable]);

  const totalPages = useMemo(
    () => Math.max(1, Math.ceil((rowsPage?.total || 0) / pageSize)),
    [pageSize, rowsPage?.total],
  );

  /** selectTable 切换当前表并重置依赖该表的查询状态。 */
  function selectTable(table: string) {
    // 1. 切换表时清空筛选和页码，避免把旧条件带入新表。
    setSelectedTable(table);
    setSearchInput("");
    setSearch("");
    setPage(1);
  }

  /** exportTable 导出当前表和搜索条件对应的脱敏 CSV。 */
  async function exportTable() {
    // 1. 导出当前筛选并把结果反馈交给统一通知组件。
    if (!selectedTable) {
      return;
    }
    setIsExporting(true);
    try {
      await downloadDatabaseTable(selectedTable, search);
      notify.success("导出完成", `${selectedTable}.csv`);
    } catch (error) {
      notify.errorFrom(error, "导出数据库表失败");
    } finally {
      setIsExporting(false);
    }
  }

  return (
    <div className="space-y-4">
      <DatabaseMetrics summary={summary} isLoading={isSummaryLoading} />

      <div className="grid min-w-0 gap-4 xl:grid-cols-[16rem_minmax(0,1fr)]">
        <Card className="hidden min-w-0 xl:block">
          <CardHeader className="pb-3">
            <CardTitle>数据表</CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[calc(100vh-22rem)] min-h-[28rem]">
              <div className="space-y-1 pr-3">
                {summary?.tables.map((table) => (
                  <Button
                    key={table.name}
                    type="button"
                    variant={selectedTable === table.name ? "default" : "ghost"}
                    className="h-auto w-full justify-between px-3 py-2 text-left"
                    onClick={() => selectTable(table.name)}
                  >
                    <span className="min-w-0 truncate font-mono text-xs">{table.name}</span>
                    <Badge variant={selectedTable === table.name ? "secondary" : "outline"}>
                      {formatInteger(table.row_count)}
                    </Badge>
                  </Button>
                ))}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>

        <Card className="min-w-0">
          <CardHeader className="gap-3 border-b pb-4">
            <div className="flex flex-col justify-between gap-3 lg:flex-row lg:items-center">
              <div className="min-w-0">
                <CardTitle className="truncate font-mono">{selectedTable || "数据表"}</CardTitle>
                <p className="mt-1 text-xs text-muted-foreground">
                  {rowsPage ? `${formatInteger(rowsPage.total)} 行 · ${rowsPage.columns.length} 列` : "正在读取"}
                </p>
              </div>
              <div className="flex min-w-0 flex-1 items-center gap-2 lg:max-w-xl">
                <InputGroup>
                  <InputGroupAddon><Search /></InputGroupAddon>
                  <InputGroupInput
                    value={searchInput}
                    onChange={(event) => setSearchInput(event.target.value)}
                    placeholder="搜索当前表"
                    aria-label="搜索当前表"
                  />
                </InputGroup>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!selectedTable || isExporting}
                  onClick={() => void exportTable()}
                >
                  {isExporting ? <Spinner /> : <Download className="h-4 w-4" />}
                  <span className="hidden sm:inline">导出</span>
                </Button>
              </div>
            </div>
            <div className="xl:hidden">
              <Select value={selectedTable} onValueChange={selectTable}>
                <SelectTrigger aria-label="选择数据表">
                  <SelectValue placeholder="选择数据表" />
                </SelectTrigger>
                <SelectContent>
                  {summary?.tables.map((table) => (
                    <SelectItem key={table.name} value={table.name}>
                      {table.name} ({formatInteger(table.row_count)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </CardHeader>

          <CardContent className="min-w-0 pt-4">
            <DatabaseRowsTable page={rowsPage} isLoading={isRowsLoading} />
            <div className="mt-4 flex flex-col justify-between gap-3 border-t pt-4 sm:flex-row sm:items-center">
              <Select
                value={String(pageSize)}
                onValueChange={(value) => {
                  setPageSize(Number(value));
                  setPage(1);
                }}
              >
                <SelectTrigger className="w-28" aria-label="每页行数">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {pageSizeOptions.map((value) => (
                    <SelectItem key={value} value={String(value)}>{value} 行</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="flex items-center justify-between gap-3 sm:justify-end">
                <span className="text-xs text-muted-foreground">第 {page}/{totalPages} 页</span>
                <Pagination className="mx-0 w-auto">
                  <PaginationContent>
                    <PaginationItem>
                      <PaginationPrevious disabled={page <= 1 || isRowsLoading} onClick={() => setPage((value) => value - 1)} />
                    </PaginationItem>
                    <PaginationItem>
                      <PaginationNext disabled={page >= totalPages || isRowsLoading} onClick={() => setPage((value) => value + 1)} />
                    </PaginationItem>
                  </PaginationContent>
                </Pagination>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

/** DatabaseMetrics 展示数据库引擎、表数、行数和文件体积。 */
function DatabaseMetrics({ summary, isLoading }: { summary: DatabaseSummary | null; isLoading: boolean }) {
  // 1. 使用稳定四列指标展示 PostgreSQL 运行状态。
  const metrics = [
    {
      label: "数据库",
      value: summary ? `${summary.engine} · ${summary.journal_mode}` : "-",
      icon: Database,
    },
    { label: "数据表", value: summary ? formatInteger(summary.table_count) : "-", icon: TableProperties },
    { label: "总行数", value: summary ? formatInteger(summary.total_rows) : "-", icon: TableProperties },
    { label: "文件大小", value: summary ? formatBytes(summary.size_bytes) : "-", icon: HardDrive },
  ];
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {metrics.map((metric) => (
        <Card key={metric.label}>
          <CardContent className="flex items-center justify-between p-4">
            <div>
              <p className="text-xs text-muted-foreground">{metric.label}</p>
              {isLoading ? <Skeleton className="mt-2 h-6 w-20" /> : <p className="mt-1 font-mono text-lg font-semibold">{metric.value}</p>}
            </div>
            <metric.icon className="h-4 w-4 text-muted-foreground" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

/** DatabaseRowsTable 按后端字段定义渲染当前页的动态只读表格。 */
function DatabaseRowsTable({ page, isLoading }: { page: DatabaseRowsPage | null; isLoading: boolean }) {
  // 1. 首次读取期间保持表格区域尺寸稳定。
  if (!page && isLoading) {
    return <Skeleton className="h-[28rem] w-full" />;
  }
  if (!page || !page.rows.length) {
    return (
      <Empty className="min-h-[22rem] border-0">
        <EmptyHeader>
          <EmptyMedia><TableProperties /></EmptyMedia>
          <EmptyTitle>暂无数据</EmptyTitle>
          <EmptyDescription>当前表或筛选条件没有记录。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  // 2. 字段驱动的动态表格保留横向滚动，避免压缩长字段。
  return (
    <div className={isLoading ? "opacity-60" : undefined}>
      <Table>
        <TableHeader>
          <TableRow>
            {page.columns.map((column) => (
              <TableHead key={column.name} className="whitespace-nowrap">
                <span>{column.name}</span>
                {column.primary_key ? <Badge variant="outline" className="ml-2 px-1 py-0 text-[10px]">PK</Badge> : null}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.rows.map((row, rowIndex) => (
            <TableRow key={databaseRowKey(row, page.columns, rowIndex)}>
              {page.columns.map((column) => (
                <TableCell key={column.name} className="max-w-[28rem] whitespace-nowrap font-mono text-xs">
                  <DatabaseValue value={row[column.name]} column={column} />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

/** DatabaseValue 统一显示空值、敏感值和可悬停查看的普通值。 */
function DatabaseValue({ value, column }: { value: unknown; column: DatabaseColumn }) {
  // 1. 空值和敏感值使用明确但紧凑的视觉状态。
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground">NULL</span>;
  }
  if (column.sensitive) {
    return <Badge variant="secondary">已隐藏</Badge>;
  }

  // 2. 普通值单行截断，完整文本保留在原生悬停提示中。
  const text = typeof value === "string" ? value : JSON.stringify(value);
  return <span className="block max-w-[28rem] truncate" title={text}>{text}</span>;
}

/** databaseRowKey 使用主键组合生成 React 行键，缺少主键时回退到页内序号。 */
function databaseRowKey(row: Record<string, unknown>, columns: DatabaseColumn[], fallback: number) {
  // 1. 优先组合主键值，缺少主键时使用当前页稳定序号。
  const primaryValues = columns.filter((column) => column.primary_key).map((column) => String(row[column.name]));
  return primaryValues.length ? primaryValues.join(":") : String(fallback);
}

/** formatInteger 把整数格式化为中文千位分隔文本。 */
function formatInteger(value: number) {
  // 1. 使用当前中文环境的千位分隔格式。
  return new Intl.NumberFormat("zh-CN").format(value);
}

/** formatBytes 把字节数转换为紧凑的人类可读单位。 */
function formatBytes(value: number) {
  // 1. 按二进制单位选择紧凑文件大小。
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}
