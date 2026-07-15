import { ApiError } from "@/lib/finance";

export type WorkNavigationLink = {
  title: string;
  url: string;
  host: string;
};

export type WorkNavigationGroup = {
  title: string;
  links: WorkNavigationLink[];
};

export type WorkNavigationData = {
  title: string;
  description: string;
  groups: WorkNavigationGroup[];
  total: number;
  is_configured: boolean;
};

export async function fetchWorkNavigation(): Promise<WorkNavigationData> {
  const response = await fetch("/api/v1/work/navigation");

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new ApiError(data?.detail || "获取工作导航数据失败", response.status);
  }

  return (await response.json()) as WorkNavigationData;
}
