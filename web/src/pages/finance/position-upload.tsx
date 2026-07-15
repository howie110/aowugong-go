import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, ImageUp, RefreshCw, UploadCloud } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { clearToken, getToken } from "@/lib/auth";
import { notify } from "@/lib/notify";

type AssetSnapshot = {
  id: number;
  snapshot_date: string;
  broker_name: string;
  source_app: string;
  account_suffix: string;
  account_alias?: string | null;
  ocr_provider?: string | null;
  warnings?: string[];
  created_at?: string | null;
  updated_at?: string | null;
};

type UploadResult = {
  filename: string;
  status: string;
  snapshot?: AssetSnapshot | null;
  error?: string | null;
};

type UploadResponse = {
  snapshot_date: string;
  results: UploadResult[];
};

function getTodayText() {
  const now = new Date();
  const offset = now.getTimezoneOffset() * 60000;
  return new Date(now.getTime() - offset).toISOString().slice(0, 10);
}

async function authorizedFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    clearToken();
    window.location.href = "/login";
  }
  return response;
}

export function PositionUploadPage() {
  const [snapshotDate, setSnapshotDate] = useState(getTodayText());
  const [files, setFiles] = useState<File[]>([]);
  const [results, setResults] = useState<UploadResult[]>([]);
  const [recent, setRecent] = useState<AssetSnapshot[]>([]);
  const [isUploading, setIsUploading] = useState(false);
  const [isLoadingRecent, setIsLoadingRecent] = useState(false);

  const selectedNames = useMemo(() => files.map((file) => file.name).join("，"), [files]);

  useEffect(() => {
    loadRecent();
  }, []);

  async function loadRecent() {
    setIsLoadingRecent(true);
    try {
      const response = await authorizedFetch("/api/v1/finance/positions/snapshots/recent?limit=20");
      if (!response.ok) {
        throw new Error("读取最近记录失败");
      }
      setRecent((await response.json()) as AssetSnapshot[]);
    } catch (error) {
      notify.errorFrom(error, "读取最近记录失败", "加载失败");
    } finally {
      setIsLoadingRecent(false);
    }
  }

  async function handleUpload() {
    if (!files.length) {
      notify.warning("请选择截图");
      return;
    }

    setIsUploading(true);
    try {
      const formData = new FormData();
      formData.set("snapshot_date", snapshotDate);
      formData.set("broker_name", "东莞证券");
      formData.set("source_app", "同花顺");
      files.forEach((file) => formData.append("files", file));

      const response = await authorizedFetch("/api/v1/finance/positions/snapshots/upload", {
        method: "POST",
        body: formData,
      });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(data?.detail || "上传识别失败");
      }
      const data = (await response.json()) as UploadResponse;
      setResults(data.results);
      setFiles([]);
      notify.success("识别保存完成", `${data.results.filter((item) => item.status === "saved").length} 张已保存。`);
      await loadRecent();
    } catch (error) {
      notify.errorFrom(error, "上传识别失败");
    } finally {
      setIsUploading(false);
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>截图上传</CardTitle>
          <CardDescription>东莞证券 / 同花顺</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 md:grid-cols-[180px_1fr_auto] md:items-end">
            <div className="space-y-2">
              <Label htmlFor="snapshot-date">日期</Label>
              <Input id="snapshot-date" type="date" value={snapshotDate} onChange={(event) => setSnapshotDate(event.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="position-images">截图</Label>
              <Input
                id="position-images"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                multiple
                onChange={(event) => setFiles(Array.from(event.target.files || []))}
              />
            </div>
            <Button type="button" disabled={isUploading} onClick={handleUpload} className="w-full md:w-auto">
              {isUploading ? <RefreshCw className="h-4 w-4 animate-spin" /> : <UploadCloud className="h-4 w-4" />}
              识别保存
            </Button>
          </div>
          {selectedNames ? (
            <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm text-muted-foreground">
              <ImageUp className="h-4 w-4 shrink-0" />
              <span className="truncate">{selectedNames}</span>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {results.length ? <UploadResultCard results={results} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>导入记录</CardTitle>
            <CardDescription>{isLoadingRecent ? "刷新中" : `${recent.length} 条`}</CardDescription>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={loadRecent} disabled={isLoadingRecent}>
            <RefreshCw className={["h-4 w-4", isLoadingRecent ? "animate-spin" : ""].join(" ")} />
            刷新
          </Button>
        </CardHeader>
        <CardContent>
          <SnapshotTable snapshots={recent} />
        </CardContent>
      </Card>
    </div>
  );
}

function UploadResultCard({ results }: { results: UploadResult[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>本次结果</CardTitle>
        <CardDescription>{results.filter((item) => item.status === "saved").length} 张已保存</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {results.map((item) => (
          <div key={item.filename} className="flex flex-col items-start justify-between gap-3 rounded-md border px-3 py-2 text-sm sm:flex-row sm:items-center">
            <div className="min-w-0">
              <div className="truncate font-medium">{item.snapshot?.account_alias || item.filename}</div>
              <div className="mt-1 truncate text-xs text-muted-foreground">
                {item.snapshot ? `${item.snapshot.snapshot_date} / ${item.snapshot.source_app}` : item.error}
              </div>
            </div>
            {item.status === "saved" ? (
              <Badge variant="success" className="shrink-0">
                <CheckCircle2 className="h-3 w-3" />
                已保存
              </Badge>
            ) : (
              <Badge variant="danger" className="shrink-0">
                失败
              </Badge>
            )}
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function SnapshotTable({ snapshots }: { snapshots: AssetSnapshot[] }) {
  if (!snapshots.length) {
    return <div className="rounded-md border px-3 py-6 text-center text-sm text-muted-foreground">暂无记录</div>;
  }

  return (
    <>
      <div className="space-y-3 md:hidden">
        {snapshots.map((item) => (
          <div key={item.id} className="rounded-md border px-3 py-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{item.account_alias || `**${item.account_suffix}`}</div>
                <div className="mt-1 text-xs text-muted-foreground">{item.snapshot_date}</div>
              </div>
              <Badge variant="outline" className="shrink-0">
                {item.source_app}
              </Badge>
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2">
              <ImportMeta label="券商" value={item.broker_name} />
              <ImportMeta label="OCR" value={item.ocr_provider || "-"} />
              <ImportMeta label="导入时间" value={formatDateTime(item.created_at || item.updated_at)} />
              <ImportMeta label="状态" value={item.warnings?.length ? `${item.warnings.length} 个提示` : "已导入"} />
            </div>
          </div>
        ))}
      </div>
      <div className="hidden overflow-x-auto md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>日期</TableHead>
              <TableHead>账户</TableHead>
              <TableHead>券商</TableHead>
              <TableHead>来源</TableHead>
              <TableHead>OCR</TableHead>
              <TableHead>导入时间</TableHead>
              <TableHead className="text-right">状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {snapshots.map((item) => (
              <TableRow key={item.id}>
                <TableCell className="whitespace-nowrap">{item.snapshot_date}</TableCell>
                <TableCell className="whitespace-nowrap">{item.account_alias || `**${item.account_suffix}`}</TableCell>
                <TableCell className="whitespace-nowrap">{item.broker_name}</TableCell>
                <TableCell className="whitespace-nowrap">{item.source_app}</TableCell>
                <TableCell className="whitespace-nowrap">{item.ocr_provider || "-"}</TableCell>
                <TableCell className="whitespace-nowrap text-muted-foreground">{formatDateTime(item.created_at || item.updated_at)}</TableCell>
                <TableCell className="text-right">
                  {item.warnings?.length ? <Badge variant="secondary">{item.warnings.length} 个提示</Badge> : <Badge variant="success">已导入</Badge>}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}

function ImportMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-muted/60 px-2 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 break-words text-sm font-medium">{value}</div>
    </div>
  );
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
