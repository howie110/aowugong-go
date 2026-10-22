import type { UserProfile } from "@/lib/auth";
import { requestJSON } from "@/lib/request";

export type ChatFilter = { source?: string; from?: string; to?: string; title?: string; q?: string; offset?: number; limit?: number };
export type ChatConversation = { id: number; source: string; source_id: string; account_scope: string; title: string; created_at: string; updated_at: string };
export type ChatMessage = { id: number; message_key: string; version_hash: string; parent_key: string | null; role: string; body: string; blocks: unknown[]; attachments: unknown[]; completeness: string; identity_quality: string; source_time: string | null; batch_id: number; position: number };
const base = "/api/v1/chat-archive/conversations";

export type ChatAuditFilter = {
 source?: string; from?: string; to?: string; q?: string;
 conversation_id?: string; message_id?: string; order?: "asc" | "desc";
 offset?: number; limit?: number; snapshot?: number;
};
export type ChatAuditMessage = {
 id: number; message_key: string; source_id: string; source: string;
 role: string; body: string; source_time: string | null; device_id: string; observed_at: string;
};
export type ChatAuditPage = {
 messages: ChatAuditMessage[];
 counts: { conversations: number; messages: number; failed_sync_states: number };
 total: number; has_more: boolean; snapshot: number;
};

export function listChatAudit(filter: ChatAuditFilter = {}) {
 const query = new URLSearchParams();
 for (const [key, value] of Object.entries(filter)) if (value !== undefined && value !== "") query.set(key, String(value));
 return requestJSON<ChatAuditPage>(`/api/v1/chat-archive/messages?${query}`, {}, "读取归档消息失败");
}

export type ChatDailyCount = { date: string; user: number; assistant: number };

export type ChatStatistics = { snapshot: number; totals: {user:number;assistant:number}; unknown:{user:number;assistant:number}; days: ChatDailyCount[]; before:string; has_more:boolean };
export function listChatStatistics(snapshot:number,before?:string) {
 const query=new URLSearchParams({snapshot:String(snapshot)});if(before)query.set("before",before);
 return requestJSON<ChatStatistics>(`/api/v1/chat-archive/statistics?${query}`,{},"读取归档统计失败");
}

export function canViewChatArchive(user: Pick<UserProfile, "roles">) {
  return (user.roles ?? []).includes("admin");
}

export function listChatConversations(filter: ChatFilter = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(filter)) if (value !== undefined && value !== "") query.set(key, String(value));
  return requestJSON<ChatConversation[]>(`${base}?${query}`, {}, "读取会话失败");
}

export function listChatMessages(id: number, after = 0) {
  return requestJSON<ChatMessage[]>(`${base}/${id}/messages?after=${after}&limit=100`, {}, "读取消息失败");
}

export function listChatVersions(id: number, key: string, after = 0) {
  return requestJSON<ChatMessage[]>(`${base}/${id}/versions?${new URLSearchParams({ message_key: key, after: String(after), limit: "100" })}`, {}, "读取版本失败");
}
