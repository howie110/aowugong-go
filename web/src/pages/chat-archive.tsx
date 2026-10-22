import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { listChatAudit, listChatStatistics, type ChatStatistics, type ChatAuditPage } from "@/lib/chat-archive";

const pageSize = 20;
const rowColumns = "grid grid-cols-[170px_80px_100px_minmax(120px,1fr)_48px] items-center gap-3";

function displayTime(value: string | null) {
  if (!value || Number.isNaN(Date.parse(value))) return "来源时间未知";
  return new Intl.DateTimeFormat("sv-SE", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" }).format(new Date(value));
}

export function ChatArchivePage() {
  const [stats, setStats] = useState<ChatStatistics | null>(null);
  const [statsBusy,setStatsBusy]=useState(false);
  const statsSequence=useRef(0);
  const daily=stats?.days;
  const [dailyError, setDailyError] = useState("");
  const [page, setPage] = useState<ChatAuditPage | null>(null);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [copyStatus, setCopyStatus] = useState("");
  const sequence = useRef(0);
  const mounted = useRef(true);
  const totals = stats?.totals;
  async function loadStatistics(snapshot:number,before?:string) {
    const token=++statsSequence.current;setStatsBusy(true);setDailyError("");
    try {const result=await listChatStatistics(snapshot,before);if(mounted.current&&token===statsSequence.current)setStats(result);}
    catch {if(mounted.current&&token===statsSequence.current)setDailyError("统计读取失败，请重试");}
    finally {if(mounted.current&&token===statsSequence.current)setStatsBusy(false);}
  }

  // 搜索建立新快照，翻页沿用已应用条件；旧请求不能覆盖较新的结果。
  async function search(nextOffset: number, snapshot?: number) {
    const token = ++sequence.current;
    setBusy(true); setError("");
    const current = () => mounted.current && token === sequence.current;
    try {
      const result = await listChatAudit({ order: "desc", offset: nextOffset, limit: pageSize, snapshot });
      if (!current()) return;
      setPage(result); setOffset(nextOffset);
      if (snapshot === undefined) {
        void loadStatistics(result.snapshot);
      }
    } catch (e) {
      if (current()) setError(e instanceof Error ? e.message : "读取归档失败");
    } finally { if (current()) setBusy(false); }
  }

  useEffect(() => {
    mounted.current = true;
    void search(0);
    return () => { mounted.current = false; sequence.current++; statsSequence.current++; };
  }, []);

  async function copyID(label: string, value: string) {
    try { await navigator.clipboard.writeText(value); setCopyStatus(`已复制 ${label}`); }
    catch { setCopyStatus("复制失败，请手动选择并复制 ID"); }
  }

  function identity(label: string, value: string) {
    return <span className="flex min-w-0 items-start gap-2"><span className="break-all">{label}: {value || "未知"}</span>{value && <Button type="button" variant="ghost" size="sm" className="h-6 shrink-0 px-2" aria-label={`复制 ${label} ${value}`} onClick={() => copyID(label, value)}>复制</Button>}</span>;
  }

  return <section className="space-y-5" aria-busy={busy}>
    <p className="text-sm text-muted-foreground">按北京时间核对每日消息数量，默认最近 7 天；总量包含全部日期。</p>
    <dl aria-label="消息总量统计" className="grid grid-cols-3 divide-x rounded-md border py-2 text-sm tabular-nums">
      <div className="px-3"><dt className="text-xs text-muted-foreground">总消息数</dt><dd className="mt-1 font-semibold">{totals ? totals.user + totals.assistant : "—"}</dd></div>
      <div className="px-3"><dt className="text-xs text-muted-foreground">user 总数</dt><dd className="mt-1 font-semibold">{totals ? totals.user : "—"}</dd></div>
      <div className="px-3"><dt className="text-xs text-muted-foreground">assistant 总数</dt><dd className="mt-1 font-semibold">{totals ? totals.assistant : "—"}</dd></div>
    </dl>
    <div className="max-h-64 overflow-auto rounded-md border" aria-label="每日消息统计">
      <table className="w-full text-left text-sm tabular-nums">
        <thead className="sticky top-0 bg-background"><tr className="border-b"><th className="px-3 py-2 font-medium">日期（北京时间）</th><th className="px-3 py-2 font-medium">user 句数</th><th className="px-3 py-2 font-medium">assistant 句数</th></tr></thead>
        <tbody>{daily?.map(day => <tr key={day.date} className="border-b last:border-0"><td className="px-3 py-1.5">{day.date}</td><td className="px-3 py-1.5">{day.user}</td><td className="px-3 py-1.5">{day.assistant}</td></tr>)}</tbody>
      </table>
      {dailyError && <p role="alert" className="p-3 text-sm text-destructive">{dailyError}</p>}
      {statsBusy && <p className="p-3 text-sm text-muted-foreground">正在读取统计…</p>}
      <div className="flex items-center gap-3 border-t p-2 text-xs">
        <Button size="sm" variant="outline" disabled={statsBusy||!page} onClick={()=>loadStatistics(page!.snapshot)}>最近 7 天</Button>
        <Button size="sm" variant="outline" disabled={statsBusy||!stats?.has_more} onClick={()=>loadStatistics(stats!.snapshot,stats!.before)}>更早 7 天</Button>
        {stats && <span>未知日期：user {stats.unknown.user} · assistant {stats.unknown.assistant}</span>}
      </div>
    </div>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <p role="status" className={copyStatus ? "text-sm text-muted-foreground" : "sr-only"}>{copyStatus}</p>
    <div className="overflow-x-auto rounded-md border">
      <div className="min-w-[680px] text-sm">
      <div className={`${rowColumns} border-b bg-muted/50 px-3 py-2 text-xs text-muted-foreground`}>
        <span>聊天时间（北京时间）</span><span>角色</span><span>Agent</span><span>内容预览</span><span>详情</span>
      </div>
      {page?.messages.map(message => <details key={message.id} className="group border-b last:border-b-0">
          <summary className={`${rowColumns} cursor-pointer list-none px-3 py-2 hover:bg-muted/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring [&::-webkit-details-marker]:hidden`}>
            <span className="whitespace-nowrap tabular-nums" title={message.source_time || "来源时间未知"}>{displayTime(message.source_time)}</span>
            <span className="truncate">{message.role}</span>
            <span className="truncate">{message.source}</span>
            <span className="truncate text-muted-foreground">{message.body.replace(/\s+/g, " ") || "（空正文）"}</span>
            <span className="text-xs text-muted-foreground"><span className="group-open:hidden">展开</span><span className="hidden group-open:inline">收起</span></span>
          </summary>
        <div className="space-y-3 border-t bg-muted/20 p-3">
          <pre className="whitespace-pre-wrap break-words font-sans text-sm [overflow-wrap:anywhere]">{message.body}</pre>
          <div className="grid gap-1 text-xs text-muted-foreground">
          {identity("message_key", message.message_key)}
          {identity("source_id", message.source_id)}
          {identity("device_id", message.device_id)}
          <span className="break-all">observed_at: {message.observed_at}</span>
        </div>
        </div>
      </details>)}
      {page && !page.messages.length && !busy && <p className="p-3 text-sm text-muted-foreground">暂无符合条件的归档消息</p>}
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-3">
      <Button variant="outline" disabled={busy || offset === 0} onClick={() => search(Math.max(0, offset - pageSize), page?.snapshot)}>上一页</Button>
      <span className="text-sm text-muted-foreground">第 {Math.floor(offset / pageSize) + 1} 页 · 匹配 {page?.total ?? 0} 条消息</span>
      <Button variant="outline" disabled={busy || !page?.has_more} onClick={() => search(offset + pageSize, page?.snapshot)}>下一页</Button>
    </div>
  </section>;
}
