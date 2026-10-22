import { requestJSON } from "@/lib/request";
export type ReceiptCounts = {
  analysis_users: number;
  tasks: number;
  succeeded: number;
  failed: number;
  running: number;
  queued: number;
  bill_users: number;
  bills: number;
};
export type ReceiptDay = ReceiptCounts & { date: string };
export type ReceiptReport = {
  version: number;
  from: string;
  to: string;
  exclude_operators: boolean;
  operators_configured: boolean;
  generated_at: string;
  lifetime_login_users: number;
  totals: ReceiptCounts;
  days: ReceiptDay[];
};
export function shiftReceiptDate(date: string, days: number) {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
export function receiptRange(days: number, now = new Date()) {
  const today = new Date(now.getTime() + 8 * 3600000)
    .toISOString()
    .slice(0, 10);
  return {
    from: shiftReceiptDate(today, 1 - days),
    to: shiftReceiptDate(today, 1),
  };
}
export function getReceiptStatistics(
  from: string,
  to: string,
  excludeOperators: boolean,
  signal?: AbortSignal,
) {
  const q = new URLSearchParams({
    from,
    to,
    exclude_operators: String(excludeOperators),
  });
  return requestJSON<ReceiptReport>(
    `/api/v1/receipt-split/statistics?${q}`,
    { signal },
    "读取小票分账统计失败",
  );
}
