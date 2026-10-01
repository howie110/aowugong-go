import { useEffect, useState } from "react";
import { getArticle, listArticles, listStatuses, type Article, type BlogStatus } from "@/lib/blog";
import { Lightbox, StatusCard } from "./blog/status-card";
import "./blog/styles.css";
import "./blog/prose.css";
import { StickerWall } from "./blog/sticker-wall";

const links = [["/blog", "文章"], ["/blog/status", "动态"], ["/blog/tags", "标签"], ["/blog/posts/blog-2000-01-04", "项目"], ["/blog/posts/blog-2000-01-03", "咖啡"], ["/blog/posts/blog-2000-01-02", "关于"]];
export function BlogPage() {
  const path = decodeURIComponent(window.location.pathname.replace(/\/+$/, ""));
  const isStatus = path === "/blog/status";
  const slug = path.startsWith("/blog/posts/") ? path.slice("/blog/posts/".length) : "";
  const tag = path.startsWith("/blog/tags/") ? path.slice("/blog/tags/".length) : "";
  const [articles, setArticles] = useState<Article[]>([]);
  const [article, setArticle] = useState<Article | null>(null);
  const [statuses, setStatuses] = useState<BlogStatus[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [zoom, setZoom] = useState("");
  const [dark, setDark] = useState(() => localStorage.getItem("blog-theme") === "dark");
  useEffect(() => {
    let active = true;
    const request = isStatus ? listStatuses().then((rows) => { if (active) { setStatuses(rows); setMore(rows.length === 20); } })
      : slug ? getArticle(slug).then((item) => { if (active) { setArticle(item); document.title = `${item.title} · 嗷呜公`; } })
        : listArticles().then((rows) => { if (active) setArticles(rows); });
    request.catch((e) => { if (active) setError(e.message); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [isStatus, slug]);
  useEffect(() => {
    if (!article) return;
    document.querySelectorAll<HTMLAnchorElement>(".blog-prose a[href]").forEach((link) => {
      if (link.getAttribute("href")?.startsWith("#")) return;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
    });
    const buttons: HTMLButtonElement[] = [];
    document.querySelectorAll<HTMLElement>(".blog-prose pre").forEach((pre) => {
      const button = document.createElement("button"); button.className = "blog-copy"; button.textContent = "复制";
      button.onclick = async () => { try { await navigator.clipboard.writeText(pre.querySelector("code")?.textContent || ""); button.textContent = "已复制"; } catch { button.textContent = "复制失败"; } };
      pre.appendChild(button); buttons.push(button);
    });
    return () => buttons.forEach((button) => button.remove());
  }, [article]);
  const tags = [...new Set(articles.flatMap((item) => item.tags))];
  async function loadMore() { setLoading(true); setError(""); try { const rows = await listStatuses(false, statuses.length); setStatuses((current) => [...current, ...rows]); setMore(rows.length === 20); } catch (e) { setError((e as Error).message); } finally { setLoading(false); } }
  return <div className="blog-public" data-theme={dark ? "dark" : "light"}>
    <div className="blog-layout">
      <aside className="blog-sidebar"><a className="blog-brand" href="/blog"><img src="/blog-static/avatar.png" alt="嗷呜公头像" /><span>嗷呜公</span></a><p className="blog-subtitle">熬一些鸡汤</p>
        <nav aria-label="博客导航">{links.map(([href, label]) => <a key={href} href={href} className={path === href || (href === "/blog" && slug && !slug.startsWith("blog-2000")) ? "active" : ""}>{label}</a>)}</nav>
        <button className="blog-theme" onClick={() => { setDark(!dark); localStorage.setItem("blog-theme", dark ? "light" : "dark"); }}>{dark ? "☀ 浅色" : "☾ 深色"}</button>
        <div className="blog-sidebar-links"><a href="/blog/rss.xml">RSS ↗</a><a href="https://github.com/howie110" target="_blank" rel="noopener noreferrer">GitHub ↗</a></div>
        {!slug && !isStatus && !tag && path === "/blog" && <StickerWall />}
      </aside>
      <main className="blog-main">
        {!slug && <header className="blog-page-header"><p className="blog-eyebrow">{isStatus ? "生活的片刻" : "文字与记录"}</p><h1>{isStatus ? "动态" : path.startsWith("/blog/tags") ? tag || "标签" : "文章"}</h1><p>{isStatus ? "随手记下，这一刻的想法。" : "对世界的观察、理解、思考与实验。"}</p></header>}
        {error && <p className="blog-error" role="alert">{error}</p>}
        {loading && !statuses.length && <p className="blog-muted" role="status">正在读取…</p>}
        {isStatus && <><div>{statuses.map((status) => <StatusCard key={status.id} status={status} />)}</div>{!loading && !error && !statuses.length && <p className="blog-muted">还没有发布动态。</p>}{more && <button className="blog-more" disabled={loading} onClick={loadMore}>{loading ? "读取中…" : "更早的动态"}</button>}</>}
        {article && <article><header className="blog-article-header"><a href="/blog" className="blog-back">← 所有文章</a><h1>{article.title}</h1><time>{article.date}</time><div className="blog-tags">{article.tags.map((tag) => <a href={`/blog/tags/${encodeURIComponent(tag)}`} key={tag}>{tag}</a>)}</div></header>
          {!!article.toc?.length && <details className="blog-toc"><summary>文章目录</summary>{article.toc.map((heading) => <a key={heading.id} href={`#${heading.id}`} style={{ paddingLeft: `${Math.max(0, heading.level - 2) * 12}px` }}>{heading.text}</a>)}</details>}
          <div className="blog-prose" onClick={(event) => { if (event.target instanceof HTMLImageElement) setZoom(event.target.src); }} dangerouslySetInnerHTML={{ __html: article.html || "" }} />
        </article>}
        {!slug && !isStatus && <>{path === "/blog/tags" && <div className="blog-tag-cloud">{tags.map((tag) => <a href={`/blog/tags/${encodeURIComponent(tag)}`} key={tag}>{tag}<span>{articles.filter((a) => a.tags.includes(tag)).length}</span></a>)}</div>}
          {articles.filter((item) => !item.slug.startsWith("blog-2000") && (!tag || item.tags.includes(tag))).map((item) => <a className="blog-article-row" href={`/blog/posts/${item.slug.split("/").map(encodeURIComponent).join("/")}`} key={item.slug}><time>{item.date}</time><div><h2>{item.title}</h2><p>{item.description}</p><div className="blog-tags">{item.tags.map((tag) => <span key={tag}>{tag}</span>)}</div></div><span className="blog-arrow">↗</span></a>)}</>}
        <footer className="blog-footer">嗷呜公 · 记录自己的生活 <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">备案信息见主站</a></footer>
      </main>
    </div>{zoom && <Lightbox url={zoom} onClose={() => setZoom("")} />}
  </div>;
}
