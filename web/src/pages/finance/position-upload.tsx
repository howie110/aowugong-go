import { useEffect, useRef, useState } from "react";
import { CheckCircle2, FileImage, ImageUp, RefreshCw, UploadCloud, X } from "lucide-react";

import { DatePicker } from "@/components/date-picker";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { authorizedFetch } from "@/lib/auth";
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

export function PositionUploadPage() {
  const [snapshotDate, setSnapshotDate] = useState(getTodayText());
  const [files, setFiles] = useState<File[]>([]);
  const [results, setResults] = useState<UploadResult[]>([]);
  const [recent, setRecent] = useState<AssetSnapshot[]>([]);
  const [isUploading, setIsUploading] = useState(false);
  const [isLoadingRecent, setIsLoadingRecent] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

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
      clearFiles();
      notify.success("识别保存完成", `${data.results.filter((item) => item.status === "saved").length} 张已保存。`);
      await loadRecent();
    } catch (error) {
      notify.errorFrom(error, "上传识别失败");
    } finally {
      setIsUploading(false);
    }
  }

  /** clearFiles 清空待上传截图及浏览器文件输入，无网络副作用。 */
  function clearFiles() {
    // 1. 清空页面附件状态。
    setFiles([]);
    // 2. 重置原生文件选择值，允许重新选择同名文件。
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  /** removeFile 从待上传列表移除指定截图，无网络副作用。 */
  function removeFile(index: number) {
    // 1. 按索引过滤目标文件。
    setFiles((current) => current.filter((_, currentIndex) => currentIndex !== index));
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
              <DatePicker id="snapshot-date" value={snapshotDate} onChange={setSnapshotDate} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="position-images">截图</Label>
              <input
                ref={fileInputRef}
                id="position-images"
                type="file"
                className="sr-only"
                accept="image/png,image/jpeg,image/webp"
                multiple
                onChange={(event) => setFiles(Array.from(event.target.files || []))}
              />
              <Button type="button" variant="outline" className="w-full justify-start" onClick={() => fileInputRef.current?.click()}>
                <ImageUp className="h-4 w-4" />
                {files.length ? `已选择 ${files.length} 张` : "选择截图"}
              </Button>
            </div>
            <Button type="button" disabled={isUploading} onClick={handleUpload} className="w-full md:w-auto">
              {isUploading ? <Spinner /> : <UploadCloud className="h-4 w-4" />}
              识别保存
            </Button>
          </div>
          {files.length ? (
            <AttachmentGroup>
              {files.map((file, index) => (
                <Attachment key={`${file.name}-${file.lastModified}`} size="sm">
                  <AttachmentMedia><FileImage /></AttachmentMedia>
                  <AttachmentContent>
                    <AttachmentTitle>{file.name}</AttachmentTitle>
                    <AttachmentDescription>{formatFileSize(file.size)}</AttachmentDescription>
                  </AttachmentContent>
                  <AttachmentActions>
                    <AttachmentAction type="button" onClick={() => removeFile(index)} aria-label={`移除 ${file.name}`}>
                      <X />
                    </AttachmentAction>
                  </AttachmentActions>
                </Attachment>
              ))}
            </AttachmentGroup>
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
            {isLoadingRecent ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
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
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>暂无导入记录</EmptyTitle>
          <EmptyDescription>上传持仓截图后会显示识别结果。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  return (
    <>
      <div className="space-y-3 md:hidden">
        {snapshots.map((item) => (
          <div key={item.id} className="rounded-md border px-3 py-3">
            <div className="flex items-start justify-between gap-3">
              <div className="text-sm font-medium">{item.snapshot_date}</div>
              <Badge variant="outline" className="shrink-0">
                {item.source_app}
              </Badge>
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2">
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

/** formatFileSize 将文件字节数格式化为易读文本，无副作用。 */
function formatFileSize(bytes: number) {
  // 1. 小于 1MB 时显示 KB，其余显示 MB。
  if (bytes < 1024 * 1024) {
    return `${Math.max(bytes / 1024, 0.1).toFixed(1)} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
