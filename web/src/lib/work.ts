import { requestJSON } from "@/lib/request";

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
  return requestJSON<WorkNavigationData>("/api/v1/work/navigation", {}, "获取工作导航数据失败");
}
