import { useState, type ReactNode } from "react";
import { splitStatusLinks, type BlogStatus } from "@/lib/blog";
import { blogImageURL } from "@/lib/blog-image";

export function Lightbox({ url, onClose, optimized = false }: { url: string; onClose: () => void; optimized?: boolean }) {
  return <div className="blog-lightbox" role="dialog" aria-modal="true" aria-label="查看图片" onClick={onClose}>
    <button autoFocus onClick={onClose} aria-label="关闭图片">×</button><img src={optimized ? blogImageURL(url, "large-v1") : url} alt="状态图片大图" decoding="async" />
    {optimized && <a className="absolute bottom-4 rounded bg-black/70 px-4 py-2 text-sm text-white" href={url} target="_blank" rel="noopener noreferrer" onClick={(event) => event.stopPropagation()}>查看原图 ↗</a>}
  </div>;
}
export function StatusText({ text }: { text: string }) {
  return <div className="blog-status-text">{splitStatusLinks(text).map((part, i) => /^https?:\/\//.test(part)
    ? <a key={i} href={part} target="_blank" rel="noopener noreferrer">{part}</a> : part)}</div>;
}
export function StatusCard({ status, actions }: { status: BlogStatus; actions?: ReactNode }) {
  return <article className="blog-status-card">
    <div className="blog-status-author"><img src="/blog-static/avatar.png" alt="" /><div><strong>嗷呜公</strong><time dateTime={status.published_at || status.created_at}>{new Date(status.published_at || status.created_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false })}</time></div>{!status.published && <span className="blog-draft">草稿</span>}</div>
    <StatusImageGallery images={status.images} />
    {!!status.body && <StatusText text={status.body} />}
    {actions && <div className="blog-status-actions">{actions}</div>}
  </article>;
}

/** 前台与工作台共用：列表只取缩略图，用户点开才请求清晰大图。 */
export function StatusImageGallery({ images }: { images: BlogStatus["images"] }) {
  const [zoom, setZoom] = useState("");
  return <>
    {!!images.length && <div className={`blog-image-grid ${images.length === 1 ? "single" : ""}`}>{images.map((image, index) => <button key={`${image.key}-${index}`} onClick={() => setZoom(image.url)} aria-label="查看大图"><img src={blogImageURL(image.url, "thumb-v1")} alt={`动态图片 ${index + 1}`} loading="lazy" decoding="async" /></button>)}</div>}
    {zoom && <Lightbox url={zoom} optimized onClose={() => setZoom("")} />}
  </>;
}
