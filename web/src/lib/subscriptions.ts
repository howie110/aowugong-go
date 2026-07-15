import { clearToken, getToken } from "@/lib/auth";

export type SubscriptionRecord = {
  id: number;
  service_name: string;
  note?: string | null;
  category: string;
  annual_fee: string;
  monthly_fee: string;
  starts_on?: string | null;
  expires_on: string;
  current_status: string;
  days_until_expiry: number;
  created_by?: string | null;
  created_at?: string | null;
  updated_at?: string | null;
};

export type SubscriptionRecordPayload = {
  service_name: string;
  note: string;
  category: string;
  annual_fee: string;
  monthly_fee: string;
  starts_on?: string | null;
  expires_on: string;
};

async function authorizedFetch(input: RequestInfo | URL, init: RequestInit = {}) {
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
  return response;
}

export async function fetchSubscriptions() {
  const response = await authorizedFetch("/api/v1/subscriptions/records");
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "读取订阅记录失败");
  }
  return (await response.json()) as SubscriptionRecord[];
}

export async function createSubscription(payload: SubscriptionRecordPayload) {
  const response = await authorizedFetch("/api/v1/subscriptions/records", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "新增订阅记录失败");
  }
  return (await response.json()) as SubscriptionRecord;
}

export async function updateSubscription(recordId: number, payload: SubscriptionRecordPayload) {
  const response = await authorizedFetch(`/api/v1/subscriptions/records/${recordId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "更新订阅记录失败");
  }
  return (await response.json()) as SubscriptionRecord;
}

export async function deleteSubscription(recordId: number) {
  const response = await authorizedFetch(`/api/v1/subscriptions/records/${recordId}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "删除订阅记录失败");
  }
  return (await response.json()) as { deleted: boolean };
}
