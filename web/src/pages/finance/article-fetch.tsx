import { ArrowDownRight, BookOpenCheck, BrainCircuit, ChevronDown, FileSearch, QrCode, Rss, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  ArticleAnalysisModelSettings,
  ArticleSource,
  WeReadArticleBinding,
  WeReadLoginStatus,
  analyzePendingArticles,
  fetchArticleReport,
  fetchArticleAnalysisModelSettings,
  fetchArticleSources,
  fetchWeReadArticleBinding,
  fetchWeReadArticleQR,
  parsePendingArticles,
  pollWeReadArticleLogin,
  saveArticleAnalysisModel,
  startWeReadArticleLogin,
  syncArticles,
} from "@/lib/article-analysis";
import { notify } from "@/lib/notify";

/** ArticleFetchPage 展示投资文章抓取状态，并管理微信读书绑定和公众号范围。 */
export function ArticleFetchPage() {
  // 1. 准备页面数据、操作状态和扫码轮询状态。
  const [sources, setSources] = useState<ArticleSource[]>([]);
  const [weRead, setWeRead] = useState<WeReadArticleBinding | null>(null);
  const [modelSettings, setModelSettings] = useState<ArticleAnalysisModelSettings | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isWorking, setIsWorking] = useState(false);
  const [isBindingWorking, setIsBindingWorking] = useState(false);
  const [isModelWorking, setIsModelWorking] = useState(false);
  const [qrOpen, setQROpen] = useState(false);
  const [qrURL, setQRURL] = useState("");
  const [loginStatus, setLoginStatus] = useState<WeReadLoginStatus | null>(null);
  const pollingRef = useRef(false);

  const sourceStatus = useMemo(() => {
    const activeCount = sources.filter((source) => source.is_active).length;
    const failedCount = sources.filter((source) => source.last_fetch_status === "error").length;
    return { activeCount, failedCount };
  }, [sources]);

  /** loadData 并行加载信息源、微信读书绑定状态和模型提示词设置。 */
  async function loadData() {
    // 1. 并行读取三个互不依赖的数据块。
    setIsLoading(true);
    try {
      const [nextSources, nextWeRead, nextModelSettings] = await Promise.all([
        fetchArticleSources(),
        fetchWeReadArticleBinding(),
        fetchArticleAnalysisModelSettings(),
      ]);
      setSources(nextSources);
      setWeRead(nextWeRead);
      setModelSettings(await withPromptDetails(nextModelSettings));
    } catch (error) {
      notify.errorFrom(error, "投资文章抓取数据加载失败", "加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  useEffect(() => {
    if (!qrOpen || !loginStatus || !["waiting", "scanned"].includes(loginStatus.state)) {
      return;
    }
    const timer = window.setInterval(async () => {
      if (pollingRef.current) {
        return;
      }
      pollingRef.current = true;
      try {
        const status = await pollWeReadArticleLogin();
        setLoginStatus(status);
        if (status.state === "connected") {
          notify.success("绑定完成", "微信读书凭据已保存，书架公众号已载入。");
          setQROpen(false);
          await loadData();
        }
      } catch (error) {
        setLoginStatus({ state: "failed", message: "读取扫码状态失败" });
        notify.errorFrom(error, "确认微信读书扫码状态失败");
      } finally {
        pollingRef.current = false;
      }
    }, 2000);
    return () => window.clearInterval(timer);
  }, [loginStatus?.state, qrOpen]);

  useEffect(() => () => {
    if (qrURL) {
      URL.revokeObjectURL(qrURL);
    }
  }, [qrURL]);

  /** openWeReadLogin 创建二维码并打开微信读书扫码弹窗。 */
  async function openWeReadLogin() {
    // 1. 创建后台单登录流程并读取带鉴权的二维码图片。
    setIsBindingWorking(true);
    try {
      const status = await startWeReadArticleLogin();
      const qr = await fetchWeReadArticleQR();
      if (qrURL) {
        URL.revokeObjectURL(qrURL);
      }
      setLoginStatus(status);
      setQRURL(URL.createObjectURL(qr));
      setQROpen(true);
    } catch (error) {
      notify.errorFrom(error, "创建微信读书二维码失败");
    } finally {
      setIsBindingWorking(false);
    }
  }

  /** handleSync 手动抓取新增文章，不触发模型分析。 */
  async function handleSync() {
    // 1. 执行统一抓取流程并在完成后刷新页面数据。
    setIsWorking(true);
    try {
      const result = await syncArticles(30, false, 1);
      notify.success("抓取新增文章完成", `新增 ${result.inserted_count} 篇，更新 ${result.updated_count} 篇。`);
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "抓取文章失败");
    } finally {
      setIsWorking(false);
    }
  }

  /** handleAnalyze 分析当前库中尚未分析的文章并补齐信号概念映射。 */
  async function handleAnalyze() {
    // 1. 执行待分析批次并在完成后刷新页面数据。
    setIsWorking(true);
    try {
      const result = await analyzePendingArticles(50, true);
      notify.success(
        "分析完成",
        `成功 ${result.analyzed_count} 篇，归类 ${result.classified_alias_count} 个信号，跳过 ${result.skipped_count} 篇，失败 ${result.error_count} 篇。`,
      );
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "分析或归类文章失败");
    } finally {
      setIsWorking(false);
    }
  }

  /** handleParse 仅解析已抓取但缺少正文的文章。 */
  async function handleParse() {
    setIsWorking(true);
    try {
      const result = await parsePendingArticles(30);
      notify.success("解析完成", `成功 ${result.parsed_count} 篇，失败 ${result.error_count} 篇。`);
      await loadData();
    } catch (error) {
      notify.errorFrom(error, "解析文章失败");
    } finally {
      setIsWorking(false);
    }
  }

  /** handleModelChange 保存后续文章分析和信号归类使用的模型。 */
  async function handleModelChange(modelId: string) {
    // 1. 保存服务端设置，成功后以服务端返回值刷新当前选择。
    setIsModelWorking(true);
    try {
      const settings = await saveArticleAnalysisModel(modelId);
      setModelSettings((current) => ({
        ...settings,
        analysis_prompt: settings.analysis_prompt?.trim() || current?.analysis_prompt || "",
        prompt_version: settings.prompt_version?.trim() || current?.prompt_version || "",
      }));
      notify.success("分析模型已切换", `后续分析使用 ${settings.selected_model}。`);
    } catch (error) {
      notify.errorFrom(error, "切换文章分析模型失败");
    } finally {
      setIsModelWorking(false);
    }
  }

  // 2. 首次加载时使用稳定尺寸的骨架，避免页面跳动。
  if (isLoading && !weRead) {
    return <ArticleFetchSkeleton />;
  }

  // 3. 渲染任务操作、微信读书设置和来源状态。
  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <ButtonGroup>
          <Button type="button" variant="outline" size="sm" className="w-28" onClick={() => void handleSync()} disabled={isWorking}>
            {isWorking ? <Spinner /> : <ArrowDownRight className="h-4 w-4" />}
            爬取
          </Button>
          <Button type="button" variant="outline" size="sm" className="w-28" onClick={() => void handleParse()} disabled={isWorking}>
            {isWorking ? <Spinner /> : <FileSearch className="h-4 w-4" />}
            解析
          </Button>
          <Button type="button" size="sm" className="w-28" onClick={() => void handleAnalyze()} disabled={isWorking}>
            {isWorking ? <Spinner /> : <Sparkles className="h-4 w-4" />}
            分析
          </Button>
        </ButtonGroup>
      </div>

      <Card>
        <CardHeader>
          <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-2">
                <BrainCircuit className="h-4 w-4" />
                分析模型
              </CardTitle>
              <CardDescription>文章分析和投资信号归类使用同一模型。</CardDescription>
            </div>
            <Select
              value={modelSettings?.selected_model_id || ""}
              onValueChange={(value) => void handleModelChange(value)}
              disabled={isModelWorking || !modelSettings?.models.length}
            >
              <SelectTrigger className="w-full sm:w-72" aria-label="选择文章分析模型">
                {isModelWorking ? <Spinner /> : <SelectValue placeholder="选择模型" />}
              </SelectTrigger>
              <SelectContent>
                {modelSettings?.models.map((model) => (
                  <SelectItem key={model.id} value={model.id} disabled={!model.configured}>
                    {model.label} · {providerLabel(model.provider)}{model.configured ? "" : "（未配置）"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardHeader>
        <CardContent>
          <Collapsible className="group/prompt overflow-hidden rounded-md border">
            <div className="flex items-center justify-between gap-3 px-3 py-2">
              <div className="min-w-0">
                <div className="text-sm font-medium">当前提示词</div>
                <div className="truncate text-xs text-muted-foreground">
                  {modelSettings?.prompt_version || "暂无版本信息"}
                </div>
              </div>
              <CollapsibleTrigger asChild>
                <Button type="button" variant="ghost" size="icon" className="h-8 w-8 shrink-0" aria-label="展开或收起当前提示词">
                  <ChevronDown className="h-4 w-4 transition-transform group-data-[state=open]/prompt:rotate-180" />
                </Button>
              </CollapsibleTrigger>
            </div>
            <CollapsibleContent className="border-t">
              <pre className="whitespace-pre-wrap break-words px-3 py-4 text-sm leading-6 text-muted-foreground">
                {modelSettings?.analysis_prompt || "暂无提示词信息。"}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        </CardContent>
      </Card>

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
        <CardContent className="space-y-4">
          <div className="flex flex-col justify-between gap-3 border-b pb-4 sm:flex-row sm:items-center">
            <div className="flex min-w-0 items-start gap-3">
              <BookOpenCheck className="mt-0.5 h-4 w-4 shrink-0" />
              <div className="min-w-0">
                <div className="font-medium">微信读书绑定</div>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-sm">
                  <Badge variant={weRead?.state === "connected" ? "success" : weRead?.state === "degraded" ? "warning" : weRead?.state === "failed" ? "danger" : "secondary"}>
                    {weReadStateLabel(weRead?.state)}
                  </Badge>
                  <span className="text-muted-foreground">{weRead?.message || "尚未绑定微信读书"}</span>
                </div>
              </div>
            </div>
            <Button type="button" size="sm" className="shrink-0" onClick={() => void openWeReadLogin()} disabled={isBindingWorking}>
              {isBindingWorking ? <Spinner /> : <QrCode className="h-4 w-4" />}
              {weRead?.state === "connected" || weRead?.state === "degraded" || weRead?.state === "failed" ? "重新绑定" : "绑定微信读书"}
            </Button>
          </div>

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
              {sources.map((source) => {
                const status = sourceDisplay(source, weRead);
                return (
                  <TableRow key={source.id}>
                    <TableCell>
                      <div className="font-medium">{source.source_name}</div>
                      <div className="text-xs text-muted-foreground">{status.message || source.description}</div>
                    </TableCell>
                    <TableCell>{source.source_type}</TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(source.last_fetch_at)}</TableCell>
                    <TableCell>
                      <Badge variant={status.variant}>{status.label}</Badge>
                    </TableCell>
                  </TableRow>
                );
              })}
              {!sources.length ? (
                <TableRow>
                  <TableCell colSpan={4}>
                    <Empty className="border-0">
                      <EmptyHeader>
                        <EmptyTitle>暂无信息源</EmptyTitle>
                        <EmptyDescription>配置并启用文章来源后会显示在这里。</EmptyDescription>
                      </EmptyHeader>
                    </Empty>
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Rss className="h-4 w-4" />
            获取公众号
          </CardTitle>
          <CardDescription>已发现公众号及文章获取状态。</CardDescription>
        </CardHeader>
        <CardContent>
          {weRead?.accounts.length ? (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="min-w-40">公众号</TableHead>
                    <TableHead className="w-24 text-right">现有文章</TableHead>
                    <TableHead className="w-24 text-right">今天新增</TableHead>
                    <TableHead className="w-24 text-right">未解析</TableHead>
                    <TableHead className="w-24 text-right">未分析</TableHead>
                    <TableHead className="min-w-32">最近获取时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {weRead.accounts.map((account) => (
                    <TableRow key={account.account_id}>
                      <TableCell className="font-medium">{account.title}</TableCell>
                      <TableCell className="text-right tabular-nums">{account.article_count}</TableCell>
                      <TableCell className="text-right tabular-nums">{account.today_inserted_count}</TableCell>
                      <TableCell className="text-right tabular-nums">{account.unparsed_count}</TableCell>
                      <TableCell className="text-right tabular-nums">{account.pending_count}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(account.latest_fetched_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : (
            <Empty className="border-0 py-6">
              <EmptyHeader>
                <EmptyTitle>暂无公众号</EmptyTitle>
                <EmptyDescription>书架公众号同步后会显示在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </CardContent>
      </Card>

      <Dialog open={qrOpen} onOpenChange={setQROpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>绑定微信读书</DialogTitle>
            <DialogDescription>使用微信扫码，并在手机上确认登录微信读书。</DialogDescription>
          </DialogHeader>
          <div className="flex min-h-64 items-center justify-center rounded-md border bg-white p-4">
            {qrURL ? <img src={qrURL} alt="微信读书登录二维码" className="h-56 w-56" /> : <Spinner />}
          </div>
          <div className="flex items-center justify-center gap-2 text-sm">
            <Badge variant={loginStatus?.state === "failed" || loginStatus?.state === "expired" ? "danger" : "secondary"}>
              {weReadStateLabel(loginStatus?.state)}
            </Badge>
            <span className="text-muted-foreground">{loginStatus?.message || "等待扫码"}</span>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">关闭</Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

async function withPromptDetails(settings: ArticleAnalysisModelSettings) {
  if (settings.analysis_prompt?.trim() && settings.prompt_version?.trim()) {
    return settings;
  }
  try {
    const report = await fetchArticleReport(1, 1);
    return {
      ...settings,
      analysis_prompt: settings.analysis_prompt?.trim() || report.analysis_prompt || "",
      prompt_version: settings.prompt_version?.trim() || report.prompt_version || "",
    };
  } catch {
    return settings;
  }
}

/** ArticleFetchSkeleton 展示文章抓取页面加载占位，无副作用。 */
function ArticleFetchSkeleton() {
  // 1. 保留命令区、绑定区和来源表的主要尺寸。
  return (
    <div className="space-y-4">
      <Skeleton className="ml-auto h-8 w-56" />
      <Skeleton className="h-40" />
      <Skeleton className="h-72" />
    </div>
  );
}

/** weReadStateLabel 将微信读书连接状态转换为简短中文。 */
function weReadStateLabel(value?: WeReadArticleBinding["state"]) {
  const labels: Record<WeReadArticleBinding["state"], string> = {
    disconnected: "未绑定",
    waiting: "等待扫码",
    scanned: "已扫码",
    connected: "可用",
    degraded: "检查异常",
    expired: "二维码已过期",
    declined: "已取消",
    failed: "已失效",
  };
  return value ? labels[value] : "未绑定";
}

/** formatDate 将时间格式化为页面使用的月日时分。 */
function formatDate(value?: string | null) {
  if (!value) {
    return "暂无";
  }
  return new Date(value).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

/** providerLabel 将模型 Provider 转换为页面短标签。 */
function providerLabel(provider: string) {
  // 1. 已知 Provider 使用品牌名，未知值保留原文便于定位配置。
  if (provider === "sub2api") {
    return "Sub2API";
  }
  if (provider === "deepseek") {
    return "DeepSeek";
  }
  return provider;
}

/** sourceDisplay 根据微信读书连接状态修正来源行的展示文案。 */
function sourceDisplay(source: ArticleSource, weRead: WeReadArticleBinding | null) {
  // 1. 微信读书来源优先使用当前绑定状态，避免旧抓取状态显示错误。
  if (source.source_type === "weread" && weRead) {
    if (weRead.state === "connected") {
      return { label: "正常", message: "微信读书凭据有效", variant: "success" as const };
    }
    if (weRead.state === "degraded") {
      return { label: "warning", message: weRead.message, variant: "warning" as const };
    }
    if (weRead.state === "failed") {
      return { label: "rebind", message: weRead.message, variant: "danger" as const };
    }
    return { label: "unbound", message: weRead.message || "尚未绑定微信读书", variant: "secondary" as const };
  }

  // 2. 其他来源继续按任务结果展示。
  if (!source.is_active) {
    return { label: "disabled", message: source.last_fetch_message || source.description, variant: "secondary" as const };
  }
  if (source.last_fetch_status === "error") {
    return { label: "error", message: source.last_fetch_message || source.description, variant: "danger" as const };
  }
  return { label: source.last_fetch_status || "ready", message: source.last_fetch_message || source.description, variant: "success" as const };
}
