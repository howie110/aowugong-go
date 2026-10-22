import { requestJSON } from "@/lib/request";

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

export async function fetchSubscriptions() {
  return requestJSON<SubscriptionRecord[]>("/api/v1/subscriptions/records", {}, "读取订阅记录失败");
}

export async function createSubscription(payload: SubscriptionRecordPayload) {
  return requestJSON<SubscriptionRecord>("/api/v1/subscriptions/records", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  }, "新增订阅记录失败");
}

export async function updateSubscription(recordId: number, payload: SubscriptionRecordPayload) {
  return requestJSON<SubscriptionRecord>(`/api/v1/subscriptions/records/${recordId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  }, "更新订阅记录失败");
}

export async function deleteSubscription(recordId: number) {
  return requestJSON<{ deleted: boolean }>(`/api/v1/subscriptions/records/${recordId}`, {
    method: "DELETE",
  }, "删除订阅记录失败");
}
