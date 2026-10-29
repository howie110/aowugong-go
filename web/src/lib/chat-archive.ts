import { requestJSON } from "@/lib/request";

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
