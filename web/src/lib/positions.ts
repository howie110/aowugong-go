import { authorizedFetch } from "@/lib/auth";
import { requestJSON } from "@/lib/request";

export type AssetSnapshot = {
  id: number;
  snapshot_date: string;
  broker_name: string;
  source_app: string;
  account_suffix: string;
  account_alias?: string | null;
  ocr_provider?: string | null;
  warnings?: string[];
  created_at?: string | null;
  updated_at?: string | null;
};

export type UploadResult = {
  filename: string;
  status: string;
  snapshot?: AssetSnapshot | null;
  error?: string | null;
};

export type UploadResponse = { snapshot_date: string; results: UploadResult[] };

export async function fetchRecentSnapshots(limit = 20): Promise<AssetSnapshot[]> {
  const response = await authorizedFetch(`/api/v1/finance/positions/snapshots/recent?limit=${limit}`);
  if (!response.ok) throw new Error("读取最近记录失败");
  return (await response.json()) as AssetSnapshot[];
}

// 上传截图并触发服务端识别与保存；不要设置 Content-Type，multipart boundary 由浏览器生成。
export function uploadPositionSnapshots(input: {
  snapshotDate: string;
  brokerName: string;
  sourceApp: string;
  files: File[];
}): Promise<UploadResponse> {
  const body = new FormData();
  body.set("snapshot_date", input.snapshotDate);
  body.set("broker_name", input.brokerName);
  body.set("source_app", input.sourceApp);
  input.files.forEach((file) => body.append("files", file));
  return requestJSON<UploadResponse>("/api/v1/finance/positions/snapshots/upload", { method: "POST", body }, "上传识别失败");
}
