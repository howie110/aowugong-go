import type { ArticleItem, DistributionItem, SignalNetPoint, TargetSignalStat } from "@/lib/article-analysis";

export type SignalSortField = "net" | "total";
export type SignalSortDirection = "asc" | "desc";

export const MARKET_MOOD_CATEGORIES = [
  "very_optimistic",
  "optimistic",
  "neutral",
  "pessimistic",
  "very_pessimistic",
  "unknown",
];
export const MARKET_PREDICTION_CATEGORIES = ["up", "range", "down", "unknown"];

export function completeDistribution(items: DistributionItem[], categories: string[]) {
  const counts = new Map(items.map((item) => [item.name, item.count]));
  return categories.map((name) => ({ name, count: counts.get(name) || 0 }));
}

export function formatDistributionValue(count: number, total: number) {
  const percent = total ? Math.round((count / total) * 100) : 0;
  return `${count}/${total} ${percent}%`;
}

export function formatShortDate(value?: string | null) {
  if (!value) {
    return "未知";
  }
  return new Date(value).toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}

/** filterArticlesBySignal 按概念组或具体标的筛选文章；输入文章、可选信号和成员名，输出匹配列表，无副作用。 */
export function filterArticlesBySignal(
  articles: ArticleItem[],
  signal: TargetSignalStat | null,
  member?: string | null,
) {
  // 1. 未选择概念组时保留完整文章列表。
  if (!signal) {
    return articles;
  }

  // 2. 选择具体标的时只匹配该名称，否则使用概念组全部原始成员。
  const members = new Set(member ? [member] : signal.members?.length ? signal.members : [signal.name]);
  return articles.filter((article) => {
    return article.recommendation_names.some((name) => members.has(name)) || article.risk_names.some((name) => members.has(name));
  });
}

/** formatSignalMembers 完整连接输入的概念组原始名称并返回展示文本，无副作用。 */
export function formatSignalMembers(members: string[]) {
  // 1. 清理空名称后按后端稳定顺序完整显示。
  return members.map((name) => name.trim()).filter(Boolean).join(" / ");
}

/** sortSignals 按净数或总数排序全部概念组，并以名称稳定处理同值项。 */
export function sortSignals(signals: TargetSignalStat[], field: SignalSortField, direction: SignalSortDirection) {
  const valueOf = (signal: TargetSignalStat) =>
    field === "net" ? signal.recommendation_count - signal.risk_count : signal.count;
  return [...signals].sort((left, right) => {
    const difference = valueOf(left) - valueOf(right);
    if (difference) {
      return direction === "asc" ? difference : -difference;
    }
    return left.name.localeCompare(right.name, "zh-CN");
  });
}

/** withSignalNetHistory 补齐概念组逐日累计净数，并转换旧服务返回的单日增量曲线。 */
export function withSignalNetHistory(
  signals: TargetSignalStat[],
  articles: ArticleItem[],
  days: number,
  endDate = new Date(),
) {
  // 1. 生成固定天数的本地日期序列，并为每个概念组建立原始标的索引。
  const dates = buildDateRange(days, endDate);
  const dateKeys = new Set(dates);
  const signalByMember = new Map<string, number>();
  signals.forEach((signal, signalIndex) => {
    for (const member of signal.members || []) {
      const key = normalizeSignalName(member);
      if (key && !signalByMember.has(key)) {
        signalByMember.set(key, signalIndex);
      }
    }
  });
  const dailyCounts = signals.map(() => new Map<string, number>());

  // 2. 使用页面已经加载的完整文章列表累计每日推荐和风险，不额外请求接口。
  const addSignals = (date: string, names: string[], delta: number) => {
    if (!dateKeys.has(date)) {
      return;
    }
    for (const name of names) {
      const signalIndex = signalByMember.get(normalizeSignalName(name));
      if (signalIndex === undefined) {
        continue;
      }
      const counts = dailyCounts[signalIndex];
      counts.set(date, (counts.get(date) || 0) + delta);
    }
  };
  for (const article of articles) {
    const date = articleDate(article);
    addSignals(date, article.recommendation_names || [], 1);
    addSignals(date, article.risk_names || [], -1);
  }

  // 3. 新版累计曲线末点必然等于排行榜净数；不相等时按旧版单日增量转换。
  return signals.map((signal, signalIndex) => {
    if (signal.net_history?.length) {
      const rankNetCount = signal.recommendation_count - signal.risk_count;
      if (signal.net_history[signal.net_history.length - 1].net_count === rankNetCount) {
        return signal;
      }
      return {
        ...signal,
        net_history: accumulateNetHistory(signal.net_history, rankNetCount),
      };
    }
    const dailyHistory = dates.map((date) => ({ date, net_count: dailyCounts[signalIndex].get(date) || 0 }));
    return {
      ...signal,
      net_history: accumulateNetHistory(dailyHistory, signal.recommendation_count - signal.risk_count),
    };
  });
}

function accumulateNetHistory(points: SignalNetPoint[], finalNetCount: number) {
  const visibleNetChange = points.reduce((total, point) => total + point.net_count, 0);
  let runningNetCount = finalNetCount - visibleNetChange;
  return points.map((point) => {
    runningNetCount += point.net_count;
    return { ...point, net_count: runningNetCount };
  });
}

export function isSameSignal(left: TargetSignalStat | null, right: TargetSignalStat) {
  return Boolean(left && left.name === right.name && left.type === right.type);
}

export function getSignalToneClass(tone: "recommend" | "risk") {
  return tone === "recommend" ? "border-transparent bg-red-50 text-red-700" : "border-transparent bg-emerald-50 text-emerald-700";
}

function buildDateRange(days: number, endDate: Date) {
  const safeDays = Math.max(1, Math.trunc(days));
  const end = new Date(endDate.getFullYear(), endDate.getMonth(), endDate.getDate());
  return Array.from({ length: safeDays }, (_, index) => {
    const date = new Date(end);
    date.setDate(end.getDate() - (safeDays - index - 1));
    return formatLocalDate(date);
  });
}

function formatLocalDate(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function articleDate(article: ArticleItem) {
  const value = (article.published_at || article.created_at || "").trim();
  return value.length >= 10 ? value.slice(0, 10) : value;
}

function normalizeSignalName(value: string) {
  return value.trim().toLocaleLowerCase("zh-CN");
}
