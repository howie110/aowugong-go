import { authorizedFetch } from "@/lib/auth";

// 保留 HTTP 状态供业务接口判断兼容分支；不自动重试请求。
export async function responseError(response: Response, fallback: string): Promise<Error & { status: number }> {
  const body = await response.json().catch(() => null);
  return Object.assign(new Error(body?.detail || fallback), { status: response.status });
}

// 只负责认证 JSON 响应的解析。请求体及 Content-Type 由业务接口指定，兼容 FormData。
export async function requestJSON<T>(input: RequestInfo | URL, init: RequestInit = {}, fallback = "请求失败"): Promise<T> {
  const response = await authorizedFetch(input, init);
  if (!response.ok) {
    throw await responseError(response, fallback);
  }
  return (await response.json()) as T;
}
