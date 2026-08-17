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

export type VPNDevice = {
  id: number;
  name: string;
  profile_code: string;
  token_version: number;
  status: "draft" | "active" | "error" | "revoked";
  published_at?: string | null;
  last_error: string;
  created_at: string;
  updated_at: string;
  subscriptions: Record<string, string>;
};

export type VPNSummary = {
  distributor_configured: boolean;
  distributor_url: string;
  profiles: VPNProfile[];
  devices: VPNDevice[];
};

// fetchVPNSummary 读取 VPN 私有资源、设备和分发状态。
// 输入：无。
// 输出：返回管理员 VPN 页面完整数据。
// 副作用：请求 Go API。
export async function fetchVPNSummary(): Promise<VPNSummary> {
  // 1. 使用统一认证请求并转换失败响应。
  return requestVPN<VPNSummary>("/api/v1/vpn/summary");
}

// createVPNDevice 新增并发布一台设备订阅。
// 输入：name 是设备名，profileCode 是资源编码。
// 输出：返回已发布设备。
// 副作用：请求 Go API 写 PostgreSQL 并推送 Worker。
export async function createVPNDevice(name: string, profileCode: string): Promise<VPNDevice> {
  // 1. 提交结构化 JSON，节点正文不会经过浏览器。
  return requestVPN<VPNDevice>("/api/v1/vpn/devices", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, profile_code: profileCode }),
  });
}

// publishVPNDevice 重新发布设备当前配置。
// 输入：deviceID 是设备主键。
// 输出：返回更新后的设备。
// 副作用：请求 Go API 推送 Worker。
export async function publishVPNDevice(deviceID: number): Promise<VPNDevice> {
  // 1. 调用设备当前版本发布入口。
  return requestVPN<VPNDevice>(`/api/v1/vpn/devices/${deviceID}/publish`, { method: "POST" });
}

// rotateVPNDevice 轮换设备订阅 Token。
// 输入：deviceID 是设备主键。
// 输出：返回包含新订阅地址的设备。
// 副作用：请求 Go API 发布新地址并撤销旧地址。
export async function rotateVPNDevice(deviceID: number): Promise<VPNDevice> {
  // 1. 调用服务端原子轮换流程。
  return requestVPN<VPNDevice>(`/api/v1/vpn/devices/${deviceID}/rotate`, { method: "POST" });
}

// revokeVPNDevice 撤销设备当前订阅。
// 输入：deviceID 是设备主键。
// 输出：返回已撤销设备。
// 副作用：请求 Go API 删除 Worker KV 并写 PostgreSQL。
export async function revokeVPNDevice(deviceID: number): Promise<VPNDevice> {
  // 1. 使用 DELETE 表达撤销语义，保留本地审计记录。
  return requestVPN<VPNDevice>(`/api/v1/vpn/devices/${deviceID}`, { method: "DELETE" });
}

// fetchVPNQRCode 下载指定订阅格式二维码。
// 输入：deviceID 是设备主键，format 是客户端格式。
// 输出：返回 PNG Blob。
// 副作用：请求 Go API。
export async function fetchVPNQRCode(deviceID: number, format: string): Promise<Blob> {
  // 1. 认证读取不缓存二维码图片。
  const response = await authorizedFetch(`/api/v1/vpn/devices/${deviceID}/qr?format=${encodeURIComponent(format)}`);
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
