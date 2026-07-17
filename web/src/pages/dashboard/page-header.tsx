import { Eye, EyeOff } from "lucide-react";

import { Button } from "@/components/ui/button";
import { type FinancePageData, type FinancePageKey, getFinancePageMeta } from "@/lib/finance";

export function PageHeader({
  pageData,
  pageKey,
  isStockAnalysisMasked,
  onToggleStockAnalysisMask,
}: {
  pageData: FinancePageData | null;
  pageKey: FinancePageKey;
  isStockAnalysisMasked: boolean;
  onToggleStockAnalysisMask: () => void;
}) {
  const fallbackMeta = getFinancePageMeta(pageKey);
  const title = pageData?.title || fallbackMeta.title;
  const description = pageData?.description || fallbackMeta.description;

  return (
    <div className="flex flex-col justify-between gap-3 md:flex-row md:items-end">
      <div>
        <h1 className="text-2xl font-semibold tracking-normal">{title}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      </div>
      {pageKey === "stockAnalysis" ? (
        <div className="flex flex-wrap items-center gap-2 md:justify-end">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onToggleStockAnalysisMask}
            title={isStockAnalysisMasked ? "显示敏感数据" : "隐藏敏感数据"}
            aria-label={isStockAnalysisMasked ? "显示敏感数据" : "隐藏敏感数据"}
          >
            {isStockAnalysisMasked ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            <span className="hidden sm:inline">{isStockAnalysisMasked ? "已隐藏" : "已显示"}</span>
          </Button>
        </div>
      ) : null}
    </div>
  );
}
