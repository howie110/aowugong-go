import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { deleteStatus, listStatuses, type BlogStatus } from "@/lib/blog";
import { StatusEditor } from "./blog/status-editor";
import "./blog/styles.css";

export function BlogStatusPage() {
  const [statuses, setStatuses] = useState<BlogStatus[]>([]);
  const [editing, setEditing] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let active = true;
    async function load() {
      setLoading(true); setError("");
      try {
        const all: BlogStatus[] = [];
        for (let offset = 0; ; offset += 20) {
          const page = await listStatuses(true, offset);
          if (!active) return;
          all.push(...page);
          if (page.length < 20) break;
        }
        setStatuses(all);

      } catch (e) { if (active) setError((e as Error).message); }
      finally { if (active) setLoading(false); }
    }
    void load();
    return () => { active = false; };
  }, [reload]);
  function saved(status: BlogStatus, isEdit: boolean) {
    setStatuses((current) => [status, ...current.filter((item) => item.id !== status.id)].sort((a, b) => (b.published_at ?? b.created_at).localeCompare(a.published_at ?? a.created_at)));
    if (isEdit) setEditing(null);
    setMessage(isEdit ? "修改已保存。" : "已发布，博客动态页现在可以看到。");
  }
  async function remove(status: BlogStatus) {
    if (deleting || !window.confirm("确定删除这条动态？删除后无法恢复。")) return;
    setDeleting(status.id); setError(""); setMessage("");
    try {
      await deleteStatus(status.id);
      setStatuses((current) => current.filter((item) => item.id !== status.id)); setMessage("动态已删除。");
    } catch (e) { setError((e as Error).message); }
    finally { setDeleting(null); }
  }
  return <div className="blog-editor">
    <div className="mb-5 flex items-center justify-between gap-3"><p className="text-sm text-muted-foreground">写几句话，记录这一刻。</p><a className="text-sm underline underline-offset-4" href="/blog/status" target="_blank" rel="noopener noreferrer">查看动态 ↗</a></div>
    <fieldset disabled={loading}><StatusEditor onSaved={(status) => saved(status, false)} /></fieldset>
    {message && <div role="status" className="blog-editor-message">{message}</div>}
    <section className="mt-10 border-t pt-6" aria-label="已发布动态管理">
      <h2 className="mb-4 text-base font-semibold">我的动态</h2>
      {loading && <p role="status" className="text-sm text-muted-foreground">正在读取动态…</p>}
      {error && <div role="alert" className="blog-error">{error} <Button variant="outline" onClick={() => setReload((value) => value + 1)}>重新读取</Button></div>}
      {!loading && !error && !statuses.length && <p className="text-sm text-muted-foreground">还没有发布动态。</p>}
      {statuses.map((status) => <article key={status.id} className="blog-status-card">
        <div className="mb-4 text-xs text-muted-foreground"><time dateTime={status.published_at ?? status.created_at}>{new Date(status.published_at ?? status.created_at).toLocaleString("zh-CN")}</time>{!status.published && <span className="ml-2">未发布</span>}</div>
        {editing === status.id ? <StatusEditor initial={status} onSaved={(item) => saved(item, true)} onCancel={() => setEditing(null)} /> : <>
          {status.images.length > 0 && <div className={`blog-image-grid${status.images.length === 1 ? " single" : ""}`}>{status.images.map((image, index) => <a href={image.url} key={`${image.key}-${index}`} target="_blank" rel="noopener noreferrer"><img src={image.url} alt={`动态图片 ${index + 1}`} loading="lazy" /></a>)}</div>}
          <p className="blog-status-text">{status.body}</p>
          <div className="blog-status-actions"><Button variant="outline" disabled={!!editing || !!deleting || loading} onClick={() => { setEditing(status.id); setMessage(""); }}>编辑</Button><Button variant="outline" disabled={!!editing || !!deleting || loading} onClick={() => void remove(status)}>{deleting === status.id ? "删除中…" : "删除"}</Button></div>
        </>}
      </article>)}
    </section>
  </div>;
}
