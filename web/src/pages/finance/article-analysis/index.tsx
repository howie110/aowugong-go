import { useEffect, useMemo, useState } from "react";

import { Card, CardContent } from "@/components/ui/card";
import { getProfile, type UserProfile } from "@/lib/auth";
import {
  fetchArticleDetail,
  fetchArticleReport,
  fetchArticles,
  type ArticleAnalysisReport,
  type ArticleDetail,
  type ArticleItem,
  type TargetSignalStat,
} from "@/lib/article-analysis";
import { notify } from "@/lib/notify";
import { ArticleDetailDrawer } from "./article-detail-drawer";
import { ArticlesCard } from "./articles-card";
import { MARKET_DAYS, TARGET_DAYS } from "./page-constants";
import { buildAccountStats, filterArticlesBySignal } from "./page-utils";
import { SignalRankCard } from "./signal-rank-card";
import { MarketPanel, ModelPromptCard, MonitoredAccountsCard } from "./summary-cards";
import { useResponsiveTablePageSize } from "./use-responsive-table-page-size";

export function ArticleAnalysisPage() {
  const [report, setReport] = useState<ArticleAnalysisReport | null>(null);
  const [articles, setArticles] = useState<ArticleItem[]>([]);
  const [user, setUser] = useState<UserProfile | null>(null);
  const [selectedArticle, setSelectedArticle] = useState<ArticleDetail | null>(null);
  const [selectedSignal, setSelectedSignal] = useState<TargetSignalStat | null>(null);
  const [signalPage, setSignalPage] = useState(1);
  const [articlePage, setArticlePage] = useState(1);
  const [isLoading, setIsLoading] = useState(true);

  const signalItems = report?.signals || [];
  const signalTable = useResponsiveTablePageSize([signalItems.length]);
  const signalPageCount = Math.max(1, Math.ceil(signalItems.length / signalTable.pageSize));
  const safeSignalPage = Math.min(signalPage, signalPageCount);
  const paginatedSignals = useMemo(() => {
    const start = (safeSignalPage - 1) * signalTable.pageSize;
    return signalItems.slice(start, start + signalTable.pageSize);
  }, [signalItems, safeSignalPage, signalTable.pageSize]);

  const filteredArticles = useMemo(() => filterArticlesBySignal(articles, selectedSignal), [articles, selectedSignal]);
  const articleTable = useResponsiveTablePageSize([filteredArticles.length, selectedSignal?.name]);
  const articlePageCount = Math.max(1, Math.ceil(filteredArticles.length / articleTable.pageSize));
  const safeArticlePage = Math.min(articlePage, articlePageCount);
  const paginatedArticles = useMemo(() => {
    const start = (safeArticlePage - 1) * articleTable.pageSize;
    return filteredArticles.slice(start, start + articleTable.pageSize);
  }, [filteredArticles, safeArticlePage, articleTable.pageSize]);

  const monitoredAccounts = useMemo(() => buildAccountStats(articles), [articles]);
  const canEditPromptFeedback = Boolean(user?.roles.includes("admin"));

  async function loadData() {
    setIsLoading(true);
    try {
      const [nextReport, nextArticles, nextUser] = await Promise.all([
        fetchArticleReport(TARGET_DAYS, MARKET_DAYS),
        fetchArticles(TARGET_DAYS, 5000),
        getProfile(),
      ]);
      setReport(nextReport);
      setArticles(nextArticles);
      setUser(nextUser);
      if (selectedArticle && !nextArticles.some((article) => article.id === selectedArticle.id)) {
        setSelectedArticle(null);
      }
    } catch (error) {
      notify.errorFrom(error, "投资文章分析数据加载失败", "加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  useEffect(() => {
    setArticlePage((currentPage) => Math.min(currentPage, articlePageCount));
  }, [articlePageCount]);

  useEffect(() => {
    setSignalPage((currentPage) => Math.min(currentPage, signalPageCount));
  }, [signalPageCount]);

  async function handleSelectArticle(article: ArticleItem) {
    try {
      setSelectedArticle(await fetchArticleDetail(article.id));
    } catch (error) {
      notify.errorFrom(error, "读取文章详情失败");
    }
  }

  function handleSelectSignal(signal: TargetSignalStat | null) {
    setSelectedSignal(signal);
    setSelectedArticle(null);
    setArticlePage(1);
  }

  function handleArticleDetailChange(article: ArticleDetail) {
    setSelectedArticle(article);
  }

  if (isLoading && !report) {
    return (
      <Card>
        <CardContent className="p-6 text-sm text-muted-foreground">正在加载投资文章分析...</CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-3 [&>*]:min-w-0 sm:gap-4 xl:grid-cols-[1fr_0.42fr_0.5fr]">
        <MarketPanel report={report} />
        <MonitoredAccountsCard accounts={monitoredAccounts} articleCount={articles.length} />
        <ModelPromptCard report={report} />
      </div>

      <div className="grid gap-4 [&>*]:min-w-0 xl:h-[calc(100dvh-8rem)] xl:min-h-[28rem] xl:max-h-[calc(100dvh-8rem)] xl:grid-cols-[minmax(20rem,0.7fr)_minmax(0,1.3fr)] xl:overflow-hidden xl:[&>*]:min-h-0">
        <SignalRankCard
          title={`信号榜 · ${TARGET_DAYS}天`}
          items={paginatedSignals}
          totalCount={signalItems.length}
          currentPage={safeSignalPage}
          totalPages={signalPageCount}
          pageSize={signalTable.pageSize}
          tableRef={signalTable.tableRef}
          selectedSignal={selectedSignal}
          onPageChange={setSignalPage}
          onSelect={handleSelectSignal}
        />
        <ArticlesCard
          articles={paginatedArticles}
          totalCount={filteredArticles.length}
          currentPage={safeArticlePage}
          totalPages={articlePageCount}
          pageSize={articleTable.pageSize}
          tableRef={articleTable.tableRef}
          selectedArticleId={selectedArticle?.id}
          selectedSignal={selectedSignal}
          onClearSignal={() => handleSelectSignal(null)}
          onPageChange={setArticlePage}
          onSelect={handleSelectArticle}
        />
      </div>

      <ArticleDetailDrawer
        article={selectedArticle}
        canEditPromptFeedback={canEditPromptFeedback}
        onArticleChange={handleArticleDetailChange}
        onClose={() => setSelectedArticle(null)}
      />
    </div>
  );
}
