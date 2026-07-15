import { clearToken, getToken } from "@/lib/auth";

export type WeReadMetric = {
  label: string;
  value: string;
  detail?: string;
};

export type WeReadBook = {
  book_id: string;
  title: string;
  author: string;
  open_url?: string;
};

export type WeReadRecentBook = WeReadBook & {
  finish_reading: boolean;
  read_date: string;
};

export type WeReadProgressBook = WeReadBook & {
  progress: number;
  chapter_idx: number;
  chapter_uid: number;
  summary: string;
  reading_time_text: string;
  update_date: string;
  read_date: string;
  finish_reading: boolean;
};

export type WeReadHeatmapDay = {
  date: string;
  seconds: number;
  minutes: number;
  level: number;
};

export type WeReadHeatmap = {
  start_date: string;
  end_date: string;
  active_days: number;
  total_seconds: number;
  total_text: string;
  days: WeReadHeatmapDay[];
};

export type WeReadDashboardData = {
  summary: {
    total_read_seconds: number;
    total_read_text: string;
    read_days: number;
    note_total: number;
    note_book_total: number;
    recent_book?: WeReadRecentBook | null;
    updated_at: string;
  };
  recent_books: WeReadRecentBook[];
  progress_books: WeReadProgressBook[];
  heatmap: WeReadHeatmap;
};

export type WeReadSummaryData = {
  metrics: WeReadMetric[];
  summary: WeReadDashboardData["summary"];
  recent_books: WeReadRecentBook[];
};

export type WeReadProgressData = {
  recent_books: WeReadRecentBook[];
  progress_books: WeReadProgressBook[];
};

export type WeReadHeatmapData = {
  heatmap: WeReadHeatmap;
};

async function fetchWeReadApi<T>(url: string, fallbackMessage: string): Promise<T> {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const response = await fetch(url, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok) {
    if (response.status === 401) {
      clearToken();
      window.location.href = "/login";
    }
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || fallbackMessage);
  }

  return (await response.json()) as T;
}

export function fetchWeReadSummary(): Promise<WeReadSummaryData> {
  return fetchWeReadApi<WeReadSummaryData>("/api/v1/weread/summary", "获取微信读书指标失败");
}

export function fetchWeReadProgress(): Promise<WeReadProgressData> {
  return fetchWeReadApi<WeReadProgressData>("/api/v1/weread/progress", "获取微信读书进度失败");
}

export function fetchWeReadHeatmap(): Promise<WeReadHeatmapData> {
  return fetchWeReadApi<WeReadHeatmapData>("/api/v1/weread/heatmap", "获取微信读书热力图失败");
}
