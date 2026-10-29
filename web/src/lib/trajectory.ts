import { beijingToday, shiftBeijingDate } from "@/lib/beijing-time";
import { requestJSON } from "@/lib/request";
export type TrajectoryDevice = {
  id: string;
  name: string;
  created_at: string;
  revoked_at: string | null;
  last_received_at: string | null;
};
export type TrajectoryCredential = {
  device: TrajectoryDevice;
  token: string;
  profile_path: string;
};
export type TrajectoryPoint = {
  id: number;
  device_id: string;
  device_name: string;
  recorded_at: string;
  received_at: string;
  lat: number;
  lon: number;
  accuracy: number | null;
  altitude: number | null;
  speed: number | null;
  bearing: number | null;
  provider: string;
};
export type TrajectoryReport = {
  points: TrajectoryPoint[];
  total: number;
  today_count: number;
  last_recorded_at: string | null;
  last_received_at: string | null;
  page: number;
  page_size: number;
  snapshot: string;
};
export type TrajectoryFilter = {
  from: string;
  through: string;
  device: string;
  page: number;
  snapshot?: string;
};
const base = "/api/v1/trajectory";
export const trajectoryToday = (now = new Date()) => beijingToday(now);
export function nextTrajectoryDay(day: string) {
  return shiftBeijingDate(day, 1);
}
export function getTrajectory(f: TrajectoryFilter, signal?: AbortSignal) {
  const q = new URLSearchParams({
    from: f.from,
    to: nextTrajectoryDay(f.through),
    page: String(f.page),
  });
  if (f.device) q.set("device_id", f.device);
  if (f.snapshot) q.set("snapshot", f.snapshot);
  return requestJSON<TrajectoryReport>(
    `${base}/points?${q}`,
    { signal },
    "读取轨迹失败",
  );
}
export const getTrajectoryDevices = (signal?: AbortSignal) =>
  requestJSON<TrajectoryDevice[]>(
    `${base}/devices`,
    { signal },
    "读取设备失败",
  );
export const createTrajectoryDevice = (name: string) =>
  requestJSON<TrajectoryCredential>(
    `${base}/devices`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    },
    "创建设备失败",
  );
export const rotateTrajectoryDevice = (id: string) =>
  requestJSON<TrajectoryCredential>(
    `${base}/devices/${encodeURIComponent(id)}/rotate`,
    { method: "POST" },
    "更换密钥失败",
  );
export const revokeTrajectoryDevice = (id: string) =>
  requestJSON<{ ok: boolean }>(
    `${base}/devices/${encodeURIComponent(id)}/revoke`,
    { method: "POST" },
    "撤销设备失败",
  );
export const getTrajectoryProfile = (id: string) =>
  requestJSON<{ profile_path: string }>(
    `${base}/devices/${encodeURIComponent(id)}/profile`,
    { method: "POST" },
    "生成配置链接失败",
  );
