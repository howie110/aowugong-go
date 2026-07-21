import type { ArticleItem, TargetSignalStat } from "@/lib/article-analysis";

export type AccountStat = {
  name: string;
  count: number;
};

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

export function buildAccountStats(articles: ArticleItem[]) {
  const counts = new Map<string, number>();
  for (const article of articles) {
    const name = getArticleAccountName(article);
    if (!name) {
      continue;
    }
    counts.set(name, (counts.get(name) || 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([name, count]) => ({ name, count }))
    .sort((left, right) => right.count - left.count || left.name.localeCompare(right.name, "zh-CN"));
}

export function isSameSignal(left: TargetSignalStat | null, right: TargetSignalStat) {
  return Boolean(left && left.name === right.name && left.type === right.type);
}

export function getSignalToneClass(tone: "recommend" | "risk") {
  return tone === "recommend" ? "border-transparent bg-red-50 text-red-700" : "border-transparent bg-emerald-50 text-emerald-700";
}

function getArticleAccountName(article: ArticleItem) {
  const title = article.title.trim();
  const matched = title.match(/^[\[【]([^\]】]{1,20})[\]】]/);
  if (matched?.[1]) {
    return firstCharacter(matched[1]);
  }
  const author = (article.author || "").trim();
  if (author && author !== article.source_name) {
    return firstCharacter(author);
  }
  return "";
}

function firstCharacter(value: string) {
  return Array.from(value.trim())[0] || "";
}
