import { useEffect, useRef, useState } from "react";
import { guardUnsavedChanges } from "@/lib/navigation-guard";
import { Button } from "@/components/ui/button";
import { saveStatus, uploadStatusImage, type BlogImage } from "@/lib/blog";
import "./blog/styles.css";

type Upload = { id: string; file: File; preview: string; progress: number; image?: BlogImage; error?: string; working: boolean };
export function BlogStatusPage() {
  const [id, setID] = useState<string>(() => crypto.randomUUID());
  const [body, setBody] = useState("");
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const fileInput = useRef<HTMLInputElement>(null);
  const uploadURLs = useRef(new Set<string>());
  const uploading = uploads.some((item) => item.working);
  const hasFailedUpload = uploads.some((item) => !item.image);
  useEffect(() => () => uploadURLs.current.forEach((url) => URL.revokeObjectURL(url)), []);
  useEffect(() => { if (dirty || uploading || busy) return guardUnsavedChanges(); }, [dirty, uploading, busy]);
  function resetEditor() {
    uploadURLs.current.forEach((url) => URL.revokeObjectURL(url)); uploadURLs.current.clear();
    setID(crypto.randomUUID()); setBody(""); setUploads([]); setDirty(false);
  }
  async function submit() {
    if (busy || uploading || hasFailedUpload) return;
    setBusy(true); setMessage(""); setError("");
    try {
      await saveStatus({ id, body, images: uploads.flatMap((item) => item.image ? [item.image] : []), published: true }, true);
      resetEditor();
      setMessage("已发布，博客动态页现在可以看到。");
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
    if (uploads.length + files.length > 9) { setError("每条动态最多九张图片。"); return; }
    const invalid = Array.from(files).find((file) => file.size > 10 * 1024 * 1024);
    if (invalid) { setError(`${invalid.name}：请使用不超过 10 MiB 的 JPEG、PNG、WebP 或 GIF 图片。`); return; }
    const pending = Array.from(files).map((file) => { const preview = URL.createObjectURL(file); uploadURLs.current.add(preview); return { id: crypto.randomUUID(), file, preview, progress: 0, working: true }; });
    setError(""); setDirty(true); setUploads((current) => [...current, ...pending]);
    for (const item of pending) await upload(item);
  }
  return <div className="blog-editor">
    <div className="mb-5 flex items-center justify-between gap-3"><p className="text-sm text-muted-foreground">写几句话，记录这一刻。</p><a className="text-sm underline underline-offset-4" href="/blog/status" target="_blank" rel="noopener noreferrer">查看动态 ↗</a></div>
    <textarea aria-label="记录内容" placeholder="今天有什么想记录的？" maxLength={30000} value={body} disabled={busy} onChange={(event) => { setBody(event.target.value); setDirty(true); }} />
    <div className="blog-upload-grid">
      {uploads.map((item) => <div className="blog-upload-item" key={item.id}><img src={item.preview} alt={item.file.name} /><button disabled={busy || item.working} aria-label="移除图片" onClick={() => { URL.revokeObjectURL(item.preview); uploadURLs.current.delete(item.preview); setUploads((current) => current.filter((u) => u.id !== item.id)); }}>×</button>{item.working ? <><progress value={item.progress} max={100} /><p>上传中 {item.progress}%</p></> : item.error ? <p className="text-destructive">{item.error} <button className="underline" onClick={() => upload(item)}>重试</button></p> : <p>已上传</p>}</div>)}
    </div>
    <input ref={fileInput} type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple className="hidden" onChange={(event) => { void selectFiles(event.target.files); event.target.value = ""; }} />
    <div className="blog-editor-toolbar">
      <Button variant="outline" disabled={busy || uploading || uploads.length >= 9} onClick={() => fileInput.current?.click()}>＋ 图片</Button>
      <span className="mr-auto text-xs text-muted-foreground">{uploads.length}/9</span>
      <Button disabled={busy || uploading || hasFailedUpload || (!body.trim() && !uploads.length)} onClick={submit}>{busy ? "发布中…" : "发布"}</Button>
    </div>
    <p className="blog-editor-hint">发布后公开可见。每张图片最多 10 MiB。</p>
    {error && <div role="alert" className="blog-error">{error}</div>}{message && <div role="status" className="blog-editor-message">{message}</div>}
  </div>;
}
