import { authorizedFetch } from "@/lib/auth";

export type VPNFormat = {
  code: string;
  name: string;
};

export type VPNProfile = {
  code: string;
  name: string;
  formats: VPNFormat[];
};

export type VPNUserSubscription = {
  id: number;
  user_id: number;
  username: string;
  profile_code: string;
  token_version: number;
  status: "draft" | "active" | "error" | "revoked";
  published_at?: string | null;
  last_error: string;
  created_at: string;
  updated_at: string;
  subscriptions: Record<string, string>;
};

export type VPNUserOption = {
  id: number;
  username: string;
  email: string;
  has_subscription: boolean;
};

export type VPNSummary = {
  distributor_configured: boolean;
  distributor_url: string;
  can_manage: boolean;
  profiles: VPNProfile[];
  user_subscriptions: VPNUserSubscription[];
  users: VPNUserOption[];
};

// fetchVPNDistributionSummary 读取管理员 VPN 分配状态。
// 输入：无。
// 输出：返回全部用户、资源和分配记录。
// 副作用：请求 Go API。
export async function fetchVPNDistributionSummary(): Promise<VPNSummary> {
  // 1. 使用统一认证请求并转换失败响应。
  return requestVPN<VPNSummary>("/api/v1/vpn/distribution/summary");
}

// fetchVPNResourceSummary 读取当前登录用户获配的 VPN 资源。
// 输入：无。
// 输出：返回当前用户的订阅和对应客户端格式。
// 副作用：请求 Go API。
export async function fetchVPNResourceSummary(): Promise<VPNSummary> {
  // 1. 请求严格按当前登录用户过滤的资源接口。
  return requestVPN<VPNSummary>("/api/v1/vpn/resources/summary");
}

// createVPNUserSubscription 给登录用户开通并发布订阅。
// 输入：userID 是用户主键，profileCode 是资源编码。
// 输出：返回已发布用户订阅。
// 副作用：请求 Go API 写 PostgreSQL 并推送 Worker。
export async function createVPNUserSubscription(userID: number, profileCode: string): Promise<VPNUserSubscription> {
  // 1. 提交用户和资源编码，节点正文不会经过浏览器。
  return requestVPN<VPNUserSubscription>("/api/v1/vpn/distribution/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: userID, profile_code: profileCode }),
  });
}

// publishVPNUserSubscription 重新发布用户当前配置。
// 输入：subscriptionID 是订阅主键。
// 输出：返回更新后的用户订阅。
// 副作用：请求 Go API 推送 Worker。
export async function publishVPNUserSubscription(subscriptionID: number): Promise<VPNUserSubscription> {
  // 1. 调用用户订阅当前版本发布入口。
  return requestVPN<VPNUserSubscription>(`/api/v1/vpn/distribution/users/${subscriptionID}/publish`, { method: "POST" });
}

// rotateVPNUserSubscription 轮换用户订阅 Token。
// 输入：subscriptionID 是订阅主键。
// 输出：返回包含新订阅地址的用户订阅。
// 副作用：请求 Go API 发布新地址并撤销旧地址。
export async function rotateVPNUserSubscription(subscriptionID: number): Promise<VPNUserSubscription> {
  // 1. 调用服务端原子轮换流程。
  return requestVPN<VPNUserSubscription>(`/api/v1/vpn/distribution/users/${subscriptionID}/rotate`, { method: "POST" });
}

// revokeVPNUserSubscription 撤销用户当前订阅。
// 输入：subscriptionID 是订阅主键。
// 输出：返回已撤销用户订阅。
// 副作用：请求 Go API 删除 Worker KV 并写 PostgreSQL。
export async function revokeVPNUserSubscription(subscriptionID: number): Promise<VPNUserSubscription> {
  // 1. 使用 DELETE 表达撤销语义，保留本地审计记录。
  return requestVPN<VPNUserSubscription>(`/api/v1/vpn/distribution/users/${subscriptionID}`, { method: "DELETE" });
}

// fetchVPNQRCode 下载指定订阅格式二维码。
// 输入：deviceID 是设备主键，format 是客户端格式。
// 输出：返回 PNG Blob。
// 副作用：请求 Go API。
export async function fetchVPNQRCode(deviceID: number, format: string): Promise<Blob> {
  // 1. 认证读取不缓存二维码图片。
  const response = await authorizedFetch(`/api/v1/vpn/resources/users/${deviceID}/qr?format=${encodeURIComponent(format)}`);
  if (!response.ok) {
    throw await vpnResponseError(response, "读取订阅二维码失败");
  }
  return response.blob();
}

// requestVPN 执行 VPN JSON API 并统一解析错误。
// 输入：path 是接口地址，init 是可选请求参数。
// 输出：返回目标 JSON 类型。
// 副作用：请求 Go API。
async function requestVPN<T>(path: string, init: RequestInit = {}): Promise<T> {
  // 1. 使用工作台令牌请求并解析成功 JSON。
  const response = await authorizedFetch(path, init);
  if (!response.ok) {
    throw await vpnResponseError(response, "VPN 操作失败");
  }
  return (await response.json()) as T;
}

// vpnResponseError 从统一错误信封提取可读消息。
// 输入：response 是失败响应，fallback 是回退文案。
// 输出：返回 Error。
// 副作用：读取响应体。
async function vpnResponseError(response: Response, fallback: string): Promise<Error> {
  // 1. 优先使用服务端 detail，解析失败时保留稳定回退。
  const body = await response.json().catch(() => null);
  return new Error(body?.detail || fallback);
}
