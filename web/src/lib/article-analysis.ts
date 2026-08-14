import { authorizedFetch } from "@/lib/auth";

export type ArticleSource = {
  id: number;
  source_code: string;
  source_name: string;
  source_type: string;
  is_active: boolean;
  description?: string | null;
  last_fetch_at?: string | null;
  last_fetch_status?: string | null;
  last_fetch_message?: string | null;
};

export type ArticleSignal = {
  name: string;
  reason?: string | null;
};

export type ArticleAnalysis = {
  summary?: string | null;
  market_mood?: string | null;
  market_mood_reason?: string | null;
  market_prediction?: string | null;
  market_prediction_reason?: string | null;
  recommendations: ArticleSignal[];
  risks: ArticleSignal[];
  error_message?: string | null;
};

export type ArticleItem = {
  id: number;
  source_name: string;
  title: string;
  author?: string | null;
  published_at?: string | null;
  market_mood?: string | null;
  market_prediction?: string | null;
  recommendation_names: string[];
  risk_names: string[];
  created_at?: string | null;
};

export type ArticleDetail = ArticleItem & {
  link: string;
  prompt_feedback?: string | null;
  analysis?: ArticleAnalysis | null;
};

export type TargetSignalStat = {
  name: string;
  type: string;
  members: string[];
  member_net_counts?: Record<string, number>;
  recommendation_count: number;
  risk_count: number;
  count: number;
};

export type DistributionItem = {
  name: string;
  count: number;
};

export type ArticleAnalysisReport = {
  analysis_model: string;
  analysis_prompt: string;
  prompt_version: string;
  signals: TargetSignalStat[];
  mood_distribution: DistributionItem[];
  prediction_distribution: DistributionItem[];
};

export type SyncResult = {
  source_count: number;
  fetched_count: number;
  inserted_count: number;
  updated_count: number;
  failed_sources: Array<{ source: string; error: string }>;
  analyzed_count: number;
  classified_alias_count: number;
  skipped_count: number;
  error_count: number;
};

export type AnalysisBatchResult = {
  analyzed_count: number;
  skipped_count: number;
  error_count: number;
  items: Array<Record<string, unknown>>;
};

export type WeReadArticleAccount = {
  account_id: string;
  title: string;
  cover_url?: string | null;
  enabled: boolean;
  fetch_interval_minutes: number;
  fetch_limit: number;
  last_checked_at?: string | null;
  article_count: number;
  today_inserted_count: number;
  pending_count: number;
  latest_fetched_at?: string | null;
};

export type WeReadArticleBinding = {
  state: "disconnected" | "waiting" | "scanned" | "connected" | "degraded" | "expired" | "declined" | "failed";
  message: string;
  accounts: WeReadArticleAccount[];
};

export type WeReadLoginStatus = {
  state: WeReadArticleBinding["state"];
  message: string;
  expires_at?: string | null;
};

export type JobRunResult = {
  id: number;
  name: string;
  source: "manual" | "scheduler" | "cli";
  status: string;
  message?: string;
};

export async function fetchArticleSources() {
  return requestJson<ArticleSource[]>("/api/v1/finance/article-analysis/sources");
}

export async function fetchArticleFetchSummary() {
  return requestJson<{ title: string; description: string; metrics: Array<{ label: string; value: string; detail: string }> }>(
    "/api/v1/finance/article-analysis/fetch-summary",
  );
}

export async function fetchWeReadArticleBinding() {
  return requestJson<WeReadArticleBinding>("/api/v1/finance/article-analysis/weread");
}

export async function startWeReadArticleLogin() {
  return requestJson<WeReadLoginStatus>("/api/v1/finance/article-analysis/weread/login", { method: "POST" });
}

export async function pollWeReadArticleLogin() {
  return requestJson<WeReadLoginStatus>("/api/v1/finance/article-analysis/weread/login");
}

export async function fetchWeReadArticleQR() {
  const response = await authorizedFetch("/api/v1/finance/article-analysis/weread/login/qr.png", { cache: "no-store" });
  if (!response.ok) {
    throw new Error("读取微信读书二维码失败");
  }
  return response.blob();
}

export async function refreshWeReadArticleAccounts() {
  return requestJson<WeReadArticleAccount[]>("/api/v1/finance/article-analysis/weread/accounts/refresh", { method: "POST" });
}

export async function saveWeReadArticleAccountSettings(accountId: string, fetchIntervalMinutes: number, fetchLimit: number) {
  return requestJson<{ status: string }>(`/api/v1/finance/article-analysis/weread/accounts/${encodeURIComponent(accountId)}`, {
    method: "PATCH",
    body: JSON.stringify({ fetch_interval_minutes: fetchIntervalMinutes, fetch_limit: fetchLimit }),
  });
}

export async function fetchArticleReport(targetDays = 60, marketDays = 3) {
  return requestJson<ArticleAnalysisReport>(`/api/v1/finance/article-analysis/report?target_days=${targetDays}&market_days=${marketDays}`);
}

export async function fetchArticles(days = 60, limit = 50) {
  return requestJson<ArticleItem[]>(`/api/v1/finance/article-analysis/articles?days=${days}&limit=${limit}`);
}

export async function fetchArticleDetail(articleId: number) {
  return requestJson<ArticleDetail>(`/api/v1/finance/article-analysis/articles/${articleId}`);
}

export async function saveArticlePromptFeedback(articleId: number, promptFeedback: string) {
  const body = JSON.stringify({ prompt_feedback: promptFeedback });
  const attempts: RequestInit[] = [
    { method: "POST", body },
    { method: "PATCH", body },
  ];
  const paths = [
    `/api/v1/finance/article-analysis/articles/${articleId}/prompt-feedback`,
    `/api/v1/finance/article-analysis/articles/${articleId}/prompt-feedback/`,
  ];
  let lastError: unknown;

  for (const path of paths) {
    for (const init of attempts) {
      try {
        return await requestJson<ArticleDetail>(path, init);
      } catch (error) {
        if (!isMethodNotAllowed(error)) {
          throw error;
        }
        lastError = error;
      }
    }
  }
  throw lastError instanceof Error ? lastError : new Error("保存提示词修正意见失败");
}

export async function syncArticles(fetchLimit = 30, analyze = false, analysisLimit = 10) {
  const params = new URLSearchParams({
    fetch_limit: String(fetchLimit),
    analyze: String(analyze),
    analysis_limit: String(analysisLimit),
  });
  return requestJson<SyncResult>(`/api/v1/finance/article-analysis/sync?${params.toString()}`, {
    method: "POST",
  });
}

export async function runScheduledArticleSync() {
  return requestJson<JobRunResult>("/api/v1/finance/jobs/sync_investment_articles/run", {
    method: "POST",
  });
}

export async function analyzePendingArticles(limit = 10) {
  return requestJson<AnalysisBatchResult>(`/api/v1/finance/article-analysis/analyze?limit=${limit}`, {
    method: "POST",
  });
}

async function requestJson<T>(input: RequestInfo | URL, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await authorizedFetch(input, { ...init, headers });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    const error = new Error(data?.detail || "投资文章分析接口请求失败");
    Object.assign(error, { status: response.status });
    throw error;
  }
  return (await response.json()) as T;
}

function isMethodNotAllowed(error: unknown) {
  return error instanceof Error && (error as Error & { status?: number }).status === 405;
}
