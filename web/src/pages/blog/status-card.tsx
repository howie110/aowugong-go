import { useState, type ReactNode } from "react";
import { splitStatusLinks, type BlogStatus } from "@/lib/blog";

export function Lightbox({ url, onClose }: { url: string; onClose: () => void }) {
  return <div className="blog-lightbox" role="dialog" aria-modal="true" aria-label="查看图片" onClick={onClose}>
    <button autoFocus onClick={onClose} aria-label="关闭图片">×</button><img src={url} alt="状态图片大图" />
  </div>;
}
export function StatusText({ text }: { text: string }) {
  return <div className="blog-status-text">{splitStatusLinks(text).map((part, i) => /^https?:\/\//.test(part)
    ? <a key={i} href={part} target="_blank" rel="noopener noreferrer">{part}</a> : part)}</div>;
}
export function StatusCard({ status, actions }: { status: BlogStatus; actions?: ReactNode }) {
  const [zoom, setZoom] = useState("");
  return <article className="blog-status-card">
    <div className="blog-status-author"><img src="/blog-static/avatar.png" alt="" /><div><strong>嗷呜公</strong><time dateTime={status.published_at || status.created_at}>{new Date(status.published_at || status.created_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false })}</time></div>{!status.published && <span className="blog-draft">草稿</span>}</div>
    {!!status.images.length && <div className={`blog-image-grid ${status.images.length === 1 ? "single" : ""}`}>{status.images.map((image) => <button key={image.key} onClick={() => setZoom(image.url)} aria-label="查看大图"><img src={image.url} alt="状态配图" loading="lazy" /></button>)}</div>}
    {!!status.body && <StatusText text={status.body} />}
    {actions && <div className="blog-status-actions">{actions}</div>}
    {zoom && <Lightbox url={zoom} onClose={() => setZoom("")} />}
  </article>;
}
