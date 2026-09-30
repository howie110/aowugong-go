import { useEffect, useRef, useState } from "react";
import { guardUnsavedChanges } from "@/lib/navigation-guard";
import { Button } from "@/components/ui/button";
import { deleteStatus, listStatuses, saveStatus, uploadStatusImage, type BlogImage, type BlogStatus } from "@/lib/blog";
import { StatusCard } from "./blog/status-card";
import "./blog/styles.css";

type Upload = { id: string; file: File; preview: string; progress: number; image?: BlogImage; error?: string; working: boolean };
export function BlogStatusPage() {
  const [rows, setRows] = useState<BlogStatus[]>([]);
  const [id, setID] = useState<string>(() => crypto.randomUUID());
  const [isNew, setIsNew] = useState(true);
  const [body, setBody] = useState("");
  const [images, setImages] = useState<BlogImage[]>([]);
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const fileInput = useRef<HTMLInputElement>(null);
  const uploadURLs = useRef(new Set<string>());
  const uploading = uploads.some((item) => item.working);
  const hasFailedUpload = uploads.some((item) => !item.image);
  useEffect(() => { refresh(); return () => uploadURLs.current.forEach((url) => URL.revokeObjectURL(url)); }, []);
  useEffect(() => { if (dirty || uploading || busy) return guardUnsavedChanges(); }, [dirty, uploading, busy]);
  async function refresh(offset = 0) {
    setLoading(true);
    try { const result = await listStatuses(true, offset); setRows((previous) => offset ? [...previous, ...result] : result); setMore(result.length === 20); }
    catch (e) { setError((e as Error).message); }
    finally { setLoading(false); }
  }
  function resetEditor() {
    uploadURLs.current.forEach((url) => URL.revokeObjectURL(url)); uploadURLs.current.clear();
    setID(crypto.randomUUID()); setIsNew(true); setBody(""); setImages([]); setUploads([]); setDirty(false);
  }
  function edit(row: BlogStatus) {
    if (dirty && !window.confirm("放弃当前未保存的内容？")) return;
    resetEditor(); setID(row.id); setIsNew(false); setBody(row.body); setImages(row.images); setMessage(""); setError(""); window.scrollTo({ top: 0, behavior: "smooth" });
  }
  async function submit(published: boolean) {
    if (busy || uploading || hasFailedUpload) return;
    setBusy(true); setMessage(""); setError("");
    try {
      const result = await saveStatus({ id, body, images: [...images, ...uploads.flatMap((item) => item.image ? [item.image] : [])], published }, isNew);
      setIsNew(false); setImages(result.images); setUploads([]); setDirty(false);
      uploadURLs.current.forEach((url) => URL.revokeObjectURL(url)); uploadURLs.current.clear();
      if (published) resetEditor();
      setMessage(published ? "已发布，博客状态页现在可以看到。" : "草稿已保存，仅你在工作台可见。"); await refresh();
    } catch (e) { setError((e as Error).message); }
    finally { setBusy(false); }
  }
  async function upload(item: Upload) {
    setUploads((current) => current.map((u) => u.id === item.id ? { ...u, working: true, error: undefined, progress: 0 } : u));
    try {
      const image = await uploadStatusImage(item.file, (progress) => setUploads((current) => current.map((u) => u.id === item.id ? { ...u, progress } : u)));
      setUploads((current) => current.map((u) => u.id === item.id ? { ...u, image, working: false, progress: 100 } : u));
    } catch (e) { setUploads((current) => current.map((u) => u.id === item.id ? { ...u, error: (e as Error).message, working: false } : u)); }
  }
  async function selectFiles(files: FileList | null) {
    if (!files) return;
    if (images.length + uploads.length + files.length > 9) { setError("每条状态最多九张图片。"); return; }
    const invalid = Array.from(files).find((file) => file.size > 10 * 1024 * 1024 || !["image/jpeg", "image/png", "image/webp", "image/gif"].includes(file.type));
    if (invalid) { setError(`${invalid.name}：请使用不超过 10 MiB 的 JPEG、PNG、WebP 或 GIF 图片。`); return; }
    const pending = Array.from(files).map((file) => { const preview = URL.createObjectURL(file); uploadURLs.current.add(preview); return { id: crypto.randomUUID(), file, preview, progress: 0, working: true }; });
    setError(""); setDirty(true); setUploads((current) => [...current, ...pending]);
    for (const item of pending) await upload(item);
  }
  async function remove(row: BlogStatus) {
    if (!window.confirm(`删除这条${row.published ? "已发布状态" : "草稿"}？`)) return;
    setBusy(true); setError("");
    try { await deleteStatus(row.id); if (row.id === id) resetEditor(); setMessage("已删除。"); await refresh(); }
    catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  }
  return <div className="blog-editor">
    <div className="mb-5 flex items-center justify-between gap-3"><p className="text-sm text-muted-foreground">写几句话，记录这一刻。</p><a className="text-sm underline underline-offset-4" href="/blog/status" target="_blank" rel="noopener noreferrer">查看公开状态 ↗</a></div>
    <textarea aria-label="状态正文" placeholder="今天有什么想记录的？" maxLength={30000} value={body} disabled={busy} onChange={(event) => { setBody(event.target.value); setDirty(true); }} />
    <div className="blog-upload-grid">
      {images.map((image) => <div className="blog-upload-item" key={image.key}><img src={image.url} alt="已上传配图" /><button disabled={busy} aria-label="移除图片" onClick={() => { setImages((current) => current.filter((i) => i.key !== image.key)); setDirty(true); }}>×</button></div>)}
      {uploads.map((item) => <div className="blog-upload-item" key={item.id}><img src={item.preview} alt={item.file.name} /><button disabled={busy || item.working} aria-label="移除图片" onClick={() => { URL.revokeObjectURL(item.preview); uploadURLs.current.delete(item.preview); setUploads((current) => current.filter((u) => u.id !== item.id)); }}>×</button>{item.working ? <><progress value={item.progress} max={100} /><p>上传中 {item.progress}%</p></> : item.error ? <p className="text-destructive">{item.error} <button className="underline" onClick={() => upload(item)}>重试</button></p> : <p>已上传</p>}</div>)}
    </div>
    <input ref={fileInput} type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple className="hidden" onChange={(event) => { void selectFiles(event.target.files); event.target.value = ""; }} />
    <div className="blog-editor-toolbar">
      <Button variant="outline" disabled={busy || uploading || images.length + uploads.length >= 9} onClick={() => fileInput.current?.click()}>＋ 图片</Button>
      <span className="mr-auto text-xs text-muted-foreground">{images.length + uploads.length}/9</span>
      <Button variant="outline" disabled={busy || uploading || hasFailedUpload || (!body.trim() && !images.length && !uploads.length)} onClick={() => submit(false)}>存草稿</Button>
      <Button disabled={busy || uploading || hasFailedUpload || (!body.trim() && !images.length && !uploads.length)} onClick={() => submit(true)}>{busy ? "保存中…" : isNew ? "发布状态" : "保存并发布"}</Button>
      {!isNew && <Button variant="ghost" disabled={busy || uploading} onClick={() => { if (!dirty || window.confirm("放弃未保存的修改？")) resetEditor(); }}>取消编辑</Button>}
    </div>
    <p className="blog-editor-hint">状态公开可见；草稿仅在工作台可见。每张图片最多 10 MiB。</p>
    {error && <div role="alert" className="blog-error">{error}</div>}{message && <div role="status" className="blog-editor-message">{message}</div>}
    <h2 className="mt-10 border-b pb-3 text-base font-semibold">我的状态与草稿</h2>
    {rows.map((row) => <StatusCard key={row.id} status={row} actions={<><button disabled={busy || uploading} onClick={() => edit(row)}>编辑</button><button disabled={busy || uploading} onClick={() => remove(row)}>删除</button></>} />)}
    {loading && <p className="mt-5 text-sm text-muted-foreground">读取中…</p>}{!loading && !rows.length && <p className="mt-5 text-sm text-muted-foreground">还没有状态，从上面发布第一条吧。</p>}
    {more && <button className="blog-more" disabled={loading} onClick={() => refresh(rows.length)}>更早的状态</button>}
  </div>;
}
