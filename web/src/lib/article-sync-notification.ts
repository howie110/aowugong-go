import type { SyncResult } from "@/lib/article-analysis";

export type ArticleSyncNotification = {
  level: "success" | "warning";
  title: string;
  description: string;
};

type ArticleSyncCounts = Pick<
  SyncResult,
  "inserted_count" | "updated_count" | "analyzed_count" | "failed_sources"
>;

/** 根据抓取结果生成统一的成功或部分失败通知。 */
export function buildArticleSyncNotification(result: ArticleSyncCounts): ArticleSyncNotification {
  // 1. 先生成所有结果都需要展示的数量摘要
  const summary = `新增 ${result.inserted_count} 篇，更新 ${result.updated_count} 篇，分析 ${result.analyzed_count} 篇，失败来源 ${result.failed_sources.length} 个。`;

  // 2. 没有失败来源时返回普通成功通知
  if (!result.failed_sources.length) {
    return {
      level: "success",
      title: "抓取并分析完成",
      description: summary,
    };
  }

  // 3. 部分失败时追加来源和原因，避免页面把异常显示成成功
  const failureDetails = result.failed_sources
    .map((item) => `${item.source || "未知来源"}：${item.error || "未知错误"}`)
    .join("；");
  return {
    level: "warning",
    title: "抓取并分析完成，存在失败来源",
    description: `${summary} ${failureDetails}`,
  };
}
