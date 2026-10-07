import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { guardUnsavedChanges } from "@/lib/navigation-guard";
import { saveStatus, uploadStatusImage, type BlogStatus } from "@/lib/blog";
import { prepareStatusImages, validateStatusFile, type StatusImageItem } from "./status-images";

type Props = { initial?: BlogStatus; onSaved: (status: BlogStatus) => void; onCancel?: () => void };
export function StatusEditor({ initial, onSaved, onCancel }: Props) {
  const [id, setID] = useState(() => initial?.id ?? crypto.randomUUID());
  const [body, setBody] = useState(initial?.body ?? "");
  const [images, setImages] = useState<StatusImageItem[]>(() => (initial?.images ?? []).map((image) => ({ id: crypto.randomUUID(), preview: image.url, image })));
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState("");
  const [progress, setProgress] = useState("");
  const locked = useRef(false);
  const input = useRef<HTMLInputElement>(null);
  const urls = useRef(new Set<string>());
  useEffect(() => () => urls.current.forEach((url) => URL.revokeObjectURL(url)), []);
  useEffect(() => { if (dirty || busy) return guardUnsavedChanges(); }, [dirty, busy]);
  async function selectFiles(files: FileList | null) {
    if (!files || locked.current) return;
    const selected = Array.from(files);
    if (images.length + selected.length > 9) { setError("每条动态最多九张图片。"); return; }
    locked.current = true; setBusy(true); setError("");
    try {
      for (const file of selected) await validateStatusFile(file);
      const pending = selected.map((file) => {
        const preview = URL.createObjectURL(file); urls.current.add(preview);
        return { id: crypto.randomUUID(), file, preview };
      });
      setImages((current) => [...current, ...pending]); setDirty(true);
    } catch (e) { setError((e as Error).message); }
    finally { locked.current = false; setBusy(false); }
  }
  function remove(item: StatusImageItem) {
    if (urls.current.delete(item.preview)) URL.revokeObjectURL(item.preview);
    setImages((current) => current.filter((image) => image.id !== item.id)); setDirty(true);
  }
  function move(from: number, to: number) {
    if (to < 0 || to >= images.length) return;
    setImages((current) => { const next = [...current]; const [item] = next.splice(from, 1); next.splice(to, 0, item); return next; }); setDirty(true);
  }
  async function submit() {
    if (locked.current || (!body.trim() && !images.length)) return;
    locked.current = true; setBusy(true); setError("");
    try {
      const uploaded = await prepareStatusImages(images, uploadStatusImage,
        (itemID, image) => setImages((current) => current.map((item) => item.id === itemID ? { ...item, image } : item)),
        (index, percent) => setProgress(`上传图片 ${index + 1}/${images.length}：${percent}%`));
      setProgress("保存中…");
      const status = await saveStatus({ id, body, images: uploaded, published: initial?.published ?? true }, !initial);
      setDirty(false);
      if (!initial) {
        urls.current.forEach((url) => URL.revokeObjectURL(url)); urls.current.clear();
        setID(crypto.randomUUID()); setBody(""); setImages([]);
      }
      onSaved(status);
    } catch (e) { setError((e as Error).message); }
    finally { locked.current = false; setBusy(false); setProgress(""); }
  }
  return <div className="blog-editor">
    <textarea aria-label={initial ? "编辑动态内容" : "记录内容"} placeholder="今天有什么想记录的？" maxLength={30000} value={body} disabled={busy} onChange={(event) => { setBody(event.target.value); setDirty(true); }} />
    <div className="blog-upload-grid">
      {images.map((item, index) => <div className="blog-upload-item" key={item.id} draggable={!busy} onDragStart={(event) => event.dataTransfer.setData("text/plain", String(index))} onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); const value = event.dataTransfer.getData("text/plain"); const from = Number(value); if (!busy && /^\d+$/.test(value) && from < images.length) move(from, index); }}>
        <img src={item.preview} alt={item.file?.name ?? `图片 ${index + 1}`} draggable={false} />
        <button type="button" disabled={busy} aria-label={`移除图片 ${index + 1}`} onClick={() => remove(item)}>×</button>
        <div className="flex items-center justify-between gap-1 py-1 text-xs"><button type="button" disabled={busy || index === 0} onClick={() => move(index, index - 1)} aria-label={`图片 ${index + 1} 前移`}>← 前移</button><span>{index + 1}</span><button type="button" disabled={busy || index === images.length - 1} onClick={() => move(index, index + 1)} aria-label={`图片 ${index + 1} 后移`}>后移 →</button></div>
      </div>)}
    </div>
    <input ref={input} type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple className="hidden" onChange={(event) => { void selectFiles(event.target.files); event.target.value = ""; }} />
    <div className="blog-editor-toolbar">
      <Button variant="outline" disabled={busy || images.length >= 9} onClick={() => input.current?.click()}>＋ 图片</Button><span className="mr-auto text-xs text-muted-foreground">{images.length}/9</span>
      {onCancel && <Button variant="outline" disabled={busy} onClick={() => { if (!dirty || window.confirm("放弃这次修改？")) onCancel(); }}>取消</Button>}
      <Button disabled={busy || (!body.trim() && !images.length)} onClick={submit}>{busy ? "处理中…" : initial ? "保存" : "发布"}</Button>
    </div>
    <p className="blog-editor-hint">选图仅本地预览，{initial ? "保存" : "发布"}时上传。每张最多 10 MiB，可拖动排序或点击前移、后移。</p>
    {progress && <p role="status" className="blog-editor-hint">{progress}</p>}
    {error && <div role="alert" className="blog-error">{error}</div>}
  </div>;
}
