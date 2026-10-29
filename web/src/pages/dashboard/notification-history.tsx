import { Eye, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Pagination, PaginationContent, PaginationItem, PaginationNext, PaginationPrevious } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { type NotificationLogEntry, type NotificationLogPage, fetchNotificationLogs } from "@/lib/finance";
import { notify } from "@/lib/notify";

const pageSizeOptions = [20, 50, 100];
const statusOptions = [
  { value: "all", label: "全部状态" },
  { value: "success", label: "成功" },
  { value: "failed", label: "失败" },
  { value: "unknown", label: "未知" },
];
const categoryOptions = [
  { value: "all", label: "全部分类" },
  { value: "任务", label: "任务" },
  { value: "备份", label: "备份" },
  { value: "监控", label: "监控" },
  { value: "订阅", label: "订阅" },
  { value: "项目", label: "项目" },
  { value: "其他", label: "其他" },
];

/** NotificationHistory 展示微信通知的发送历史、筛选、分页和完整正文。 */
export function NotificationHistory() {
  const [searchInput, setSearchInput] = useState("");
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [category, setCategory] = useState("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [data, setData] = useState<NotificationLogPage | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [selected, setSelected] = useState<NotificationLogEntry | null>(null);

  useEffect(() => {
    // 1. 输入停止短暂时间后再执行服务器搜索。
    const timer = window.setTimeout(() => {
      setQuery(searchInput.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    // 1. 筛选、页码或每页数量变化时读取历史，并忽略已经过期的响应。
    let cancelled = false;
    setIsLoading(true);
    fetchNotificationLogs({
      q: query,
      status: status === "all" ? undefined : status,
      category: category === "all" ? undefined : category,
      page,
      pageSize,
    })
      .then((result) => {
        if (!cancelled) {
          setData(result);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          notify.errorFrom(error, "读取通知历史失败", "加载失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [category, page, pageSize, query, status]);

  const totalPages = useMemo(
    () => Math.max(1, Math.ceil((data?.total || 0) / pageSize)),
    [data?.total, pageSize],
  );

  /** changeStatus 切换状态筛选并回到第一页。 */
  function changeStatus(value: string) {
    setStatus(value);
    setPage(1);
  }

  /** changeCategory 切换分类筛选并回到第一页。 */
  function changeCategory(value: string) {
    setCategory(value);
    setPage(1);
  }

  /** changePageSize 切换每页数量并回到第一页。 */
  function changePageSize(value: string) {
    setPageSize(Number(value));
    setPage(1);
  }

  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle>发送历史</CardTitle>
        <CardDescription>查看已写入数据库的微信通知，支持关键字、状态和分类筛选。</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
          <InputGroup className="lg:max-w-sm">
            <InputGroupAddon><Search /></InputGroupAddon>
            <InputGroupInput
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder="搜索标题或正文"
              aria-label="搜索通知标题或正文"
            />
          </InputGroup>
          <div className="flex flex-wrap items-center gap-2">
            <Select value={status} onValueChange={changeStatus}>
              <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent>
                {statusOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={category} onValueChange={changeCategory}>
              <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent>
                {categoryOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={String(pageSize)} onValueChange={changePageSize}>
              <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={String(size)}>{size} 条/页</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {isLoading && !data ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-12" />)}
          </div>
        ) : data && data.items.length > 0 ? (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="whitespace-nowrap">时间</TableHead>
                  <TableHead className="min-w-48">标题</TableHead>
                  <TableHead className="min-w-64">内容</TableHead>
                  <TableHead className="w-24">状态</TableHead>
                  <TableHead className="w-16 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="whitespace-nowrap text-muted-foreground">{entry.sent_at}</TableCell>
                    <TableCell className="font-medium">{entry.title}</TableCell>
                    <TableCell className="max-w-[28rem] truncate text-muted-foreground" title={entry.message}>
                      {entry.message}
                    </TableCell>
                    <TableCell>{statusBadge(entry.status)}</TableCell>
                    <TableCell className="text-right">
                      <Button type="button" variant="ghost" size="sm" onClick={() => setSelected(entry)}>
                        <Eye className="h-4 w-4" />
                        <span className="hidden sm:inline">查看</span>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : (
          <Empty className="min-h-48 border-0">
            <EmptyHeader>
              <EmptyMedia><Search /></EmptyMedia>
              <EmptyTitle>没有通知记录</EmptyTitle>
              <EmptyDescription>调整关键字、状态或分类后再试。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}

        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <span className="text-sm text-muted-foreground">
            共 {data?.total || 0} 条 · 第 {page} / {totalPages} 页
          </span>
          <Pagination className="mx-0 w-auto">
            <PaginationContent>
              <PaginationItem>
                <PaginationPrevious
                  disabled={page <= 1 || isLoading}
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                />
              </PaginationItem>
              <PaginationItem>
                <PaginationNext
                  disabled={page >= totalPages || isLoading}
                  onClick={() => setPage((value) => value + 1)}
                />
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        </div>
      </CardContent>

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{selected?.title}</DialogTitle>
            <DialogDescription>
              {selected?.sent_at} · {selected?.channel}
              {selected?.source ? ` · 来源 ${selected.source}` : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div>{selected ? statusBadge(selected.status) : null}</div>
            <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-4 text-sm">
              {selected?.message}
            </pre>
            {selected?.error_message ? (
              <p className="text-sm text-destructive">发送错误：{selected.error_message}</p>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

/** statusBadge 把发送状态转换为统一徽标。 */
function statusBadge(status: string) {
  if (status === "success") {
    return <Badge variant="success">成功</Badge>;
  }
  if (status === "failed") {
    return <Badge variant="danger">失败</Badge>;
  }
  return <Badge variant="secondary">未知</Badge>;
}
