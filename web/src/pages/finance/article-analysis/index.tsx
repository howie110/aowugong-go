import { useEffect, useMemo, useState } from "react";

import { Skeleton } from "@/components/ui/skeleton";
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
import { buildAccountStats, filterArticlesBySignal, isSameSignal } from "./page-utils";
import { SignalRankCard } from "./signal-rank-card";
import { MarketPanel, ModelPromptCard, MonitoredAccountsCard } from "./summary-cards";
import { useResponsiveTablePageSize } from "./use-responsive-table-page-size";

export function ArticleAnalysisPage() {
  const [report, setReport] = useState<ArticleAnalysisReport | null>(null);
  const [articles, setArticles] = useState<ArticleItem[]>([]);
  const [user, setUser] = useState<UserProfile | null>(null);
  const [selectedArticle, setSelectedArticle] = useState<ArticleDetail | null>(null);
  const [selectedRankSignal, setSelectedRankSignal] = useState<TargetSignalStat | null>(null);
  const [selectedRankMember, setSelectedRankMember] = useState<string | null>(null);
  const [selectedArticleSignal, setSelectedArticleSignal] = useState<TargetSignalStat | null>(null);
  const [selectedArticleMember, setSelectedArticleMember] = useState<string | null>(null);
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

  const filteredArticles = useMemo(
    () => filterArticlesBySignal(articles, selectedArticleSignal, selectedArticleMember),
    [articles, selectedArticleMember, selectedArticleSignal],
  );
  const articleTable = useResponsiveTablePageSize([
    filteredArticles.length,
    selectedArticleSignal?.name,
    selectedArticleMember,
  ]);
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

  // handleSelectRankSignal 从信号榜筛选文章，并维护信号榜自身的选中状态。
  // 输入：signal 是用户点击的概念组。
  // 输出：无。
  // 副作用：更新信号榜选中项、文章筛选、文章页码和已打开文章。
  function handleSelectRankSignal(signal: TargetSignalStat) {
    // 1. 仅当两侧都停留在同一概念组且未选择具体标的时，再次点击才取消筛选。
    const nextSignal =
      isSameSignal(selectedRankSignal, signal) &&
      !selectedRankMember &&
      isSameSignal(selectedArticleSignal, signal) &&
      !selectedArticleMember
        ? null
        : signal;
    setSelectedRankSignal(nextSignal);
    setSelectedRankMember(null);
    setSelectedArticleSignal(nextSignal);
    setSelectedArticleMember(null);
    setSelectedArticle(null);
    setArticlePage(1);
  }

  // handleSelectRankMember 从信号榜概念组内筛选具体标的，并同步文章当前位置。
  // 输入：signal 是所属概念组，member 是用户点击的具体标的。
  // 输出：无。
  // 副作用：更新信号榜和文章筛选、文章页码及已打开文章。
  function handleSelectRankMember(signal: TargetSignalStat, member: string) {
    // 1. 再次点击当前具体标的时退回概念组，否则精确筛选该标的。
    const nextMember =
      isSameSignal(selectedRankSignal, signal) &&
      selectedRankMember === member &&
      isSameSignal(selectedArticleSignal, signal) &&
      selectedArticleMember === member
        ? null
        : member;
    setSelectedRankSignal(signal);
    setSelectedRankMember(nextMember);
    setSelectedArticleSignal(signal);
    setSelectedArticleMember(nextMember);
    setSelectedArticle(null);
    setArticlePage(1);
  }

  // handleArticleBreadcrumbChange 从文章面包屑返回概念组或全部文章，不联动信号榜。
  // 输入：level 是返回层级，group 保留概念组，all 清除全部筛选。
  // 输出：无。
  // 副作用：更新文章筛选、文章页码和已打开文章。
  function handleArticleBreadcrumbChange(level: "all" | "group") {
    // 1. 返回全部文章时清除概念组；两种返回操作都会清除具体标的。
    if (level === "all") {
      setSelectedArticleSignal(null);
    }
    setSelectedArticleMember(null);
    setSelectedArticle(null);
    setArticlePage(1);
  }

  function handleArticleDetailChange(article: ArticleDetail) {
    setSelectedArticle(article);
  }

  if (isLoading && !report) {
    return (
      <div className="space-y-4">
        <div className="grid gap-4 xl:grid-cols-3">
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
        </div>
        <div className="grid gap-4 xl:grid-cols-[0.7fr_1.3fr]">
          <Skeleton className="h-[32rem]" />
          <Skeleton className="h-[32rem]" />
        </div>
      </div>
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
          selectedSignal={selectedRankSignal}
          selectedMember={selectedRankMember}
          onPageChange={setSignalPage}
          onSelect={handleSelectRankSignal}
          onSelectMember={handleSelectRankMember}
        />
        <ArticlesCard
          articles={paginatedArticles}
          totalCount={filteredArticles.length}
          currentPage={safeArticlePage}
          totalPages={articlePageCount}
          pageSize={articleTable.pageSize}
          tableRef={articleTable.tableRef}
          selectedArticleId={selectedArticle?.id}
          selectedSignal={selectedArticleSignal}
          selectedMember={selectedArticleMember}
          onClearSignal={() => handleArticleBreadcrumbChange("all")}
          onSelectSignalGroup={() => handleArticleBreadcrumbChange("group")}
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
