import { Suspense, lazy, useEffect, useState } from "react";

import { AppShell } from "@/components/layout/app-shell";
import { UserProfile, getProfile } from "@/lib/auth";
import {
  ApiError,
  type FinancePageData,
  type FinancePageKey,
  fetchFinancePage,
  getFinancePageFromPath,
  getFinancePagePath,
  pagePermissionMap,
} from "@/lib/finance";
import {
  BacktestContent,
  DataContent,
  ErrorCard,
  JobsContent,
  LoadingCard,
  NotificationsContent,
  OverviewContent,
  TradingContent,
} from "./dashboard/finance-content";
import { PageHeader } from "./dashboard/page-header";

const PositionUploadPage = lazy(() => import("@/pages/finance/position-upload").then((module) => ({ default: module.PositionUploadPage })));
const StockAnalysisPage = lazy(() => import("@/pages/finance/stock-analysis").then((module) => ({ default: module.StockAnalysisPage })));
const ArticleFetchPage = lazy(() => import("@/pages/finance/article-fetch").then((module) => ({ default: module.ArticleFetchPage })));
const ArticleAnalysisPage = lazy(() => import("@/pages/finance/article-analysis").then((module) => ({ default: module.ArticleAnalysisPage })));
const WeReadPage = lazy(() => import("@/pages/weread").then((module) => ({ default: module.WeReadPage })));
const MahjongPage = lazy(() => import("@/pages/mahjong").then((module) => ({ default: module.MahjongPage })));
const SubscriptionsPage = lazy(() => import("@/pages/subscriptions").then((module) => ({ default: module.SubscriptionsPage })));
const WorkPage = lazy(() => import("@/pages/work").then((module) => ({ default: module.WorkPage })));
const PermissionsPage = lazy(() => import("@/pages/permissions").then((module) => ({ default: module.PermissionsPage })));
const MonitoringPage = lazy(() => import("@/pages/monitoring").then((module) => ({ default: module.MonitoringPage })));
const DatabasePage = lazy(() => import("@/pages/database").then((module) => ({ default: module.DatabasePage })));

const selfManagedPages = new Set<FinancePageKey>([
  "positions",
  "stockAnalysis",
  "articleFetch",
  "articleAnalysis",
  "weread",
  "mahjong",
  "subscriptions",
  "work",
  "permissions",
  "monitoring",
  "database",
]);
const pageOrder: FinancePageKey[] = [
  "overview",
  "work",
  "articleAnalysis",
  "articleFetch",
  "stockAnalysis",
  "positions",
  "weread",
  "mahjong",
  "subscriptions",
  "backtest",
  "data",
  "jobs",
  "trading",
  "notifications",
  "monitoring",
  "database",
  "permissions",
];

type DashboardPageProps = {
  initialPage: FinancePageKey;
};

export function DashboardPage({ initialPage }: DashboardPageProps) {
  const [user, setUser] = useState<UserProfile | null>(null);
  const [activePage, setActivePage] = useState<FinancePageKey>(initialPage);
  const [pageData, setPageData] = useState<FinancePageData | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);
  const [isReady, setIsReady] = useState(false);
  const [isPageLoading, setIsPageLoading] = useState(false);
  const [isStockAnalysisMasked, setIsStockAnalysisMasked] = useState(true);

  useEffect(() => {
    getProfile()
      .then(setUser)
      .catch(() => {
        window.location.href = "/login";
      })
      .finally(() => setIsReady(true));
  }, []);

  useEffect(() => {
    if (!isReady) {
      return;
    }
    let isCancelled = false;
    setPageData(null);
    setPageError(null);
    if (user && !canAccessPage(user, activePage)) {
      const fallbackPage = getFirstAccessiblePage(user);
      if (fallbackPage && fallbackPage !== activePage) {
        window.history.replaceState({}, "", getFinancePagePath(fallbackPage));
        setActivePage(fallbackPage);
        setIsPageLoading(false);
        return () => {
          isCancelled = true;
        };
      }
      setPageError("没有访问权限");
      setIsPageLoading(false);
      return () => {
        isCancelled = true;
      };
    }
    if (selfManagedPages.has(activePage)) {
      setIsPageLoading(false);
      return () => {
        isCancelled = true;
      };
    }
    setIsPageLoading(true);
    fetchFinancePage(activePage)
      .then((data) => {
        if (!isCancelled) {
          setPageData(data);
        }
      })
      .catch((error) => {
        if (isCancelled) {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          window.location.href = "/login";
          return;
        }
        setPageData(null);
        setPageError(error instanceof Error ? error.message : "获取页面数据失败");
      })
      .finally(() => {
        if (!isCancelled) {
          setIsPageLoading(false);
        }
      });
    return () => {
      isCancelled = true;
    };
  }, [activePage, isReady, user]);

  useEffect(() => {
    function handlePopState() {
      setActivePage(getFinancePageFromPath(window.location.pathname));
    }

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  useEffect(() => {
    setIsStockAnalysisMasked(true);
  }, [activePage]);

  function handleNavigate(pageKey: FinancePageKey) {
    const path = getFinancePagePath(pageKey);
    window.history.pushState({}, "", path);
    setActivePage(pageKey);
  }

  if (!isReady) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background text-sm text-muted-foreground">
        正在进入控制台...
      </div>
    );
  }

  return (
    <AppShell user={user} activePage={activePage} onNavigate={handleNavigate}>
      <div className="space-y-6 p-4 md:p-6">
        <PageHeader
          pageData={pageData}
          pageKey={activePage}
          isStockAnalysisMasked={isStockAnalysisMasked}
          onToggleStockAnalysisMask={() => setIsStockAnalysisMasked((value) => !value)}
        />
        {pageError ? (
          <ErrorCard message={pageError} />
        ) : !selfManagedPages.has(activePage) && (isPageLoading || !pageData) ? (
          <LoadingCard />
        ) : (
          <Suspense fallback={<LoadingCard />}>
            <FinancePageBody pageKey={activePage} pageData={pageData} isStockAnalysisMasked={isStockAnalysisMasked} />
          </Suspense>
        )}
      </div>
    </AppShell>
  );
}

function FinancePageBody({
  pageKey,
  pageData,
  isStockAnalysisMasked,
}: {
  pageKey: FinancePageKey;
  pageData: FinancePageData | null;
  isStockAnalysisMasked: boolean;
}) {
  if (pageKey === "overview") {
    return pageData ? <OverviewContent pageData={pageData} /> : <LoadingCard />;
  }
  if (pageKey === "weread") {
    return <WeReadPage />;
  }
  if (pageKey === "mahjong") {
    return <MahjongPage />;
  }
  if (pageKey === "subscriptions") {
    return <SubscriptionsPage />;
  }
  if (pageKey === "work") {
    return <WorkPage />;
  }
  if (pageKey === "positions") {
    return <PositionUploadPage />;
  }
  if (pageKey === "stockAnalysis") {
    return <StockAnalysisPage isSensitiveMasked={isStockAnalysisMasked} />;
  }
  if (pageKey === "articleAnalysis") {
    return <ArticleAnalysisPage />;
  }
  if (pageKey === "articleFetch") {
    return <ArticleFetchPage />;
  }
  if (pageKey === "permissions") {
    return <PermissionsPage />;
  }
  if (pageKey === "monitoring") {
    return <MonitoringPage />;
  }
  if (pageKey === "database") {
    return <DatabasePage />;
  }
  if (!pageData) {
    return <LoadingCard />;
  }
  if (pageKey === "backtest") {
    return <BacktestContent pageData={pageData} />;
  }
  if (pageKey === "data") {
    return <DataContent pageData={pageData} />;
  }
  if (pageKey === "jobs") {
    return <JobsContent pageData={pageData} />;
  }
  if (pageKey === "trading") {
    return <TradingContent pageData={pageData} />;
  }
  return <NotificationsContent pageData={pageData} />;
}

function canAccessPage(user: UserProfile, pageKey: FinancePageKey) {
  if ((user.roles ?? []).includes("admin")) {
    return true;
  }
  return (user.permissions ?? []).includes(pagePermissionMap[pageKey]);
}

function getFirstAccessiblePage(user: UserProfile) {
  return pageOrder.find((pageKey) => canAccessPage(user, pageKey));
}
