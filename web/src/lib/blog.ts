import { authorizedFetch, clearToken, getToken } from "@/lib/auth";
import { requestJSON, responseError } from "@/lib/request";

export type BlogImage = { key: string; url: string };
export type Article = { slug: string; title: string; date: string; tags: string[]; description: string; html?: string; toc: { id: string; text: string; level: number }[] | null };
export type BlogStatus = { id: string; body: string; images: BlogImage[]; published: boolean; published_at: string | null; created_at: string; updated_at: string };
export type StatusInput = Pick<BlogStatus, "id" | "body" | "images" | "published">;
const publicAPI = "/api/v1/blog";
const adminAPI = `${publicAPI}/admin`;
async function publicJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) throw await responseError(response, "读取博客失败");
  return response.json();
}
export const listArticles = () => publicJSON<Article[]>(`${publicAPI}/posts`);
export const getArticle = (slug: string) => publicJSON<Article>(`${publicAPI}/posts/${slug.split("/").map(encodeURIComponent).join("/")}`);
export const listStatuses = (admin = false, offset = 0) => admin
  ? requestJSON<BlogStatus[]>(`${adminAPI}/statuses?limit=20&offset=${offset}`)
  : publicJSON<BlogStatus[]>(`${publicAPI}/statuses?limit=20&offset=${offset}`);
export function saveStatus(input: StatusInput, create: boolean) {
  return requestJSON<BlogStatus>(`${adminAPI}/statuses${create ? "" : `/${encodeURIComponent(input.id)}`}`, {
    method: create ? "POST" : "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  }, "保存状态失败");
}
export async function deleteStatus(id: string) {
  const response = await authorizedFetch(`${adminAPI}/statuses/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!response.ok) throw await responseError(response, "删除状态失败");
}
// XHR 只用于提供真实上传进度；凭据与现有认证模块共用，不自动重试写入。
export function uploadStatusImage(file: File, onProgress: (percent: number) => void): Promise<BlogImage> {
  return new Promise((resolve, reject) => {
    const token = getToken();
    if (!token) { reject(new Error("未登录")); return; }
    const request = new XMLHttpRequest();
    request.open("POST", `${adminAPI}/images`);
    request.timeout = 90000;
    request.setRequestHeader("Authorization", `Bearer ${token}`);
    request.upload.onprogress = (event) => { if (event.lengthComputable) onProgress(Math.round(event.loaded / event.total * 100)); };
    request.onerror = () => reject(new Error("上传中断，请重试"));
    request.ontimeout = () => reject(new Error("上传超时，请重试"));
    request.onload = () => {
      if (request.status === 401) { clearToken(); window.location.href = "/login"; }
      try {
        const data = JSON.parse(request.responseText);
        if (request.status >= 200 && request.status < 300) resolve(data as BlogImage);
        else reject(new Error(data.detail || "上传失败"));
      } catch { reject(new Error("上传返回无效响应")); }
    };
    const form = new FormData(); form.append("image", file); request.send(form);
  });
}

// 中文标点是正文，不应成为网址的一部分。
export function splitStatusLinks(text: string) {
  return text.split(/(https?:\/\/[^\s<>（）。，！？、；：“”「」]+)/g);
}
