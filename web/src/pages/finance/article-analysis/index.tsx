import { useEffect, useMemo, useRef, useState } from "react";

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
import {
  filterArticlesBySignal,
  isSameSignal,
  sortSignals,
  withSignalNetHistory,
  type SignalSortDirection,
  type SignalSortField,
} from "./page-utils";
import { SignalRankCard } from "./signal-rank-card";
import { MarketPanel } from "./summary-cards";

const SIGNAL_PAGE_SIZE = 8;
const ARTICLE_PAGE_SIZE = 20;

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
  const [signalSort, setSignalSort] = useState<{ field: SignalSortField; direction: SignalSortDirection }>({
    field: "total",
    direction: "desc",
  });
  const [articlePage, setArticlePage] = useState(1);
  const [isLoading, setIsLoading] = useState(true);

  const signalItems = report?.signals || [];
  const sortedSignalItems = useMemo(
    () => sortSignals(signalItems, signalSort.field, signalSort.direction),
    [signalItems, signalSort.direction, signalSort.field],
  );
  const signalTableRef = useRef<HTMLDivElement>(null);
  const articleTableRef = useRef<HTMLDivElement>(null);
  const signalPageCount = Math.max(1, Math.ceil(sortedSignalItems.length / SIGNAL_PAGE_SIZE));
  const safeSignalPage = Math.min(signalPage, signalPageCount);
  const paginatedSignals = useMemo(() => {
    const start = (safeSignalPage - 1) * SIGNAL_PAGE_SIZE;
    return sortedSignalItems.slice(start, start + SIGNAL_PAGE_SIZE);
  }, [safeSignalPage, sortedSignalItems]);

  const filteredArticles = useMemo(
    () => filterArticlesBySignal(articles, selectedArticleSignal, selectedArticleMember),
    [articles, selectedArticleMember, selectedArticleSignal],
  );
  const articlePageCount = Math.max(1, Math.ceil(filteredArticles.length / ARTICLE_PAGE_SIZE));
  const safeArticlePage = Math.min(articlePage, articlePageCount);
  const paginatedArticles = useMemo(() => {
    const start = (safeArticlePage - 1) * ARTICLE_PAGE_SIZE;
    return filteredArticles.slice(start, start + ARTICLE_PAGE_SIZE);
  }, [filteredArticles, safeArticlePage]);

  const canEditPromptFeedback = Boolean(user?.roles.includes("admin"));

  async function loadData() {
    setIsLoading(true);
    try {
      const [nextReport, nextArticles, nextUser] = await Promise.all([
        fetchArticleReport(TARGET_DAYS, MARKET_DAYS),
        fetchArticles(TARGET_DAYS, 5000),
        getProfile(),
      ]);
      setReport({ ...nextReport, signals: withSignalNetHistory(nextReport.signals || [], nextArticles, TARGET_DAYS) });
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

  function handleSignalSort(field: SignalSortField) {
    setSignalSort((current) => ({
      field,
      direction: current.field === field && current.direction === "desc" ? "asc" : "desc",
    }));
    setSignalPage(1);
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
        <Skeleton className="h-36" />
        <Skeleton className="h-[64rem]" />
        <Skeleton className="h-[32rem]" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <MarketPanel report={report} />

      <div className="space-y-4 [&>*]:min-w-0">
        <SignalRankCard
          title={`信号榜 · ${TARGET_DAYS}天`}
          items={paginatedSignals}
          totalCount={signalItems.length}
          currentPage={safeSignalPage}
          totalPages={signalPageCount}
          pageSize={SIGNAL_PAGE_SIZE}
          tableRef={signalTableRef}
          selectedSignal={selectedRankSignal}
          selectedMember={selectedRankMember}
          sortField={signalSort.field}
          sortDirection={signalSort.direction}
          onPageChange={setSignalPage}
          onSort={handleSignalSort}
          onSelect={handleSelectRankSignal}
          onSelectMember={handleSelectRankMember}
        />
        <ArticlesCard
          articles={paginatedArticles}
          totalCount={filteredArticles.length}
          currentPage={safeArticlePage}
          totalPages={articlePageCount}
          pageSize={ARTICLE_PAGE_SIZE}
          tableRef={articleTableRef}
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
