import { clearToken, getToken } from "@/lib/auth";

export type ServiceMonitorResult = {
  target_code: string;
  target_name: string;
  target_url: string;
  status: "up" | "down" | "unknown" | string;
  http_status?: number | null;
  latency_ms?: number | null;
  error_message?: string | null;
  checked_at?: string | null;
};

export type ServiceMonitorSummary = {
  title: string;
  description: string;
  metrics: Array<{ label: string; value: string; detail: string; status?: string }>;
  services: ServiceMonitorResult[];
};

export type ServiceMonitorCheckResult = {
  checked_count: number;
  up_count: number;
  down_count: number;
  results: ServiceMonitorResult[];
};

export async function fetchMonitoringSummary() {
  return requestJson<ServiceMonitorSummary>("/api/v1/monitoring/summary");
}

export async function checkMonitoringServices() {
  return requestJson<ServiceMonitorCheckResult>("/api/v1/monitoring/check", { method: "POST" });
}

async function requestJson<T>(input: RequestInfo | URL, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    clearToken();
    window.location.href = "/login";
  }
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "获取监控数据失败");
  }
  return (await response.json()) as T;
}
