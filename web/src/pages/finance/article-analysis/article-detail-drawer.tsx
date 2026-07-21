import { ExternalLink } from "lucide-react";
import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import {
  type ArticleAnalysis,
  type ArticleDetail,
  type ArticleSignal,
  saveArticlePromptFeedback,
} from "@/lib/article-analysis";
import { notify } from "@/lib/notify";
import {
  getMarketMoodClass,
  getMarketMoodIcon,
  getMarketPredictionClass,
  getMarketPredictionIcon,
  translate,
} from "./market-ui";
import { getSignalToneClass } from "./page-utils";

/** 渲染文章分析明细抽屉。 */
export function ArticleDetailDrawer({
  article,
  canEditPromptFeedback,
  onArticleChange,
  onClose,
}: {
  article: ArticleDetail | null;
  canEditPromptFeedback: boolean;
  onArticleChange: (article: ArticleDetail) => void;
  onClose: () => void;
}) {
  const [promptFeedback, setPromptFeedback] = useState("");
  const [isSavingFeedback, setIsSavingFeedback] = useState(false);

  useEffect(() => {
    setPromptFeedback(article?.prompt_feedback || "");
  }, [article?.id, article?.prompt_feedback]);

  async function handleSavePromptFeedback() {
    if (!article || !canEditPromptFeedback) {
      return;
    }
    setIsSavingFeedback(true);
    try {
      const updatedArticle = await saveArticlePromptFeedback(article.id, promptFeedback);
      onArticleChange(updatedArticle);
      notify.success("修正意见已保存");
    } catch (error) {
      notify.errorFrom(error, "保存提示词修正意见失败");
    } finally {
      setIsSavingFeedback(false);
    }
  }

  if (!article) {
    return null;
  }
  const analysis = article.analysis;
  return (
    <Sheet open={Boolean(article)} onOpenChange={(open) => !open && onClose()}>
      <SheetContent side="left" className="!w-full !max-w-xl p-0 sm:!max-w-xl">
        <div className="flex h-full min-h-0 flex-col">
          <SheetHeader className="border-b px-5 py-4 pr-12 text-left">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <SheetTitle>文章列表明细</SheetTitle>
                <SheetDescription className="mt-1">查看分析结果并记录提示词修正意见。</SheetDescription>
              </div>
              <Button asChild variant="outline" size="sm">
                <a href={article.link} target="_blank" rel="noreferrer">
                  <ExternalLink className="h-4 w-4" />
                  原文
                </a>
              </Button>
            </div>
          </SheetHeader>
          <ScrollArea className="min-h-0 flex-1">
            <div className="space-y-3 p-5">
            {analysis ? (
              <>
                {analysis.summary ? (
                  <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">{analysis.summary}</div>
                ) : null}
                <ArticleMarketJudgmentList analysis={analysis} />
                <ArticleSignalList recommendations={analysis.recommendations} risks={analysis.risks} />
                {analysis.error_message ? (
                  <div className="rounded-md border px-3 py-2 text-sm text-destructive">{analysis.error_message}</div>
                ) : null}
                <PromptFeedbackPanel
                  value={promptFeedback}
                  isSaving={isSavingFeedback}
                  canEdit={canEditPromptFeedback}
                  onChange={setPromptFeedback}
                  onSave={() => void handleSavePromptFeedback()}
                />
              </>
            ) : (
              <>
                <div className="rounded-md border px-3 py-6 text-sm text-muted-foreground">这篇文章还没有分析结果。</div>
                <PromptFeedbackPanel
                  value={promptFeedback}
                  isSaving={isSavingFeedback}
                  canEdit={canEditPromptFeedback}
                  onChange={setPromptFeedback}
                  onSave={() => void handleSavePromptFeedback()}
                />
              </>
            )}
            </div>
          </ScrollArea>
        </div>
      </SheetContent>
    </Sheet>
  );
}

/** 渲染管理员可编辑的提示词修正意见。 */
function PromptFeedbackPanel({
  value,
  isSaving,
  canEdit,
  onChange,
  onSave,
}: {
  value: string;
  isSaving: boolean;
  canEdit: boolean;
  onChange: (value: string) => void;
  onSave: () => void;
}) {
  return (
    <div className="rounded-md border">
      <div className="flex items-center justify-between gap-3 border-b px-3 py-2">
        <div>
          <div className="text-sm font-medium">提示词修正意见</div>
          <div className="mt-1 text-xs text-muted-foreground">
            {canEdit ? "记录这篇文章暴露出的提示词问题，后续统一迭代。" : "仅管理员可修改。"}
          </div>
        </div>
        {canEdit ? (
          <Button type="button" size="sm" onClick={onSave} disabled={isSaving}>
            {isSaving ? "保存中" : "保存"}
          </Button>
        ) : null}
      </div>
      <div className="p-3">
        {canEdit ? (
          <>
            <Textarea
              value={value}
              onChange={(event) => onChange(event.target.value)}
              maxLength={4000}
              placeholder="例如：这篇文章是中长期看多，不应该只按短期情绪抽取；某个标的应合并为风险而不是推荐..."
              className="min-h-28 resize-y leading-6"
            />
            <div className="mt-1 text-right text-xs text-muted-foreground">{value.length}/4000</div>
          </>
        ) : (
          <div className="min-h-16 whitespace-pre-wrap rounded-md border bg-muted/30 px-3 py-2 text-sm leading-6 text-muted-foreground">
            {value || "暂无修正意见"}
          </div>
        )}
      </div>
    </div>
  );
}

/** 渲染文章推荐与风险信号，手机端让原因独占下一行。 */
function ArticleSignalList({ recommendations, risks }: { recommendations: ArticleSignal[]; risks: ArticleSignal[] }) {
  const signals = [
    ...recommendations.map((item) => ({ ...item, signal: "recommend" as const })),
    ...risks.map((item) => ({ ...item, signal: "risk" as const })),
  ];

  if (!signals.length) {
    return <div className="rounded-md border px-3 py-3 text-sm text-muted-foreground">未提及信号。</div>;
  }

  return (
    <div className="rounded-md border">
      <div className="border-b px-3 py-2 text-sm font-medium">信号</div>
      <div className="divide-y">
        {signals.map((item) => (
          <div
            key={`${item.signal}-${item.name}-${item.reason}`}
            data-article-signal-row
            className="grid grid-cols-[5rem_minmax(0,1fr)] items-start gap-x-2 gap-y-1.5 px-3 py-2 text-sm sm:grid-cols-[5rem_8rem_minmax(0,1fr)] sm:gap-y-0"
          >
            <Badge className={`${getSignalToneClass(item.signal)} w-fit`}>{item.signal === "recommend" ? "推荐" : "风险"}</Badge>
            <div className="truncate font-medium">{item.name}</div>
            <div data-article-signal-reason className="col-span-2 text-xs leading-relaxed text-muted-foreground sm:col-span-1">
              {item.reason || "未给出原因"}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/** 渲染市场氛围与涨跌预测，手机端让原因独占下一行。 */
function ArticleMarketJudgmentList({ analysis }: { analysis: ArticleAnalysis }) {
  const rows = [
    {
      key: "mood",
      label: "市场氛围",
      value: analysis.market_mood,
      reason: analysis.market_mood_reason,
      className: getMarketMoodClass(analysis.market_mood),
      Icon: getMarketMoodIcon(analysis.market_mood),
    },
    {
      key: "prediction",
      label: "涨跌预测",
      value: analysis.market_prediction,
      reason: analysis.market_prediction_reason,
      className: getMarketPredictionClass(analysis.market_prediction),
      Icon: getMarketPredictionIcon(analysis.market_prediction),
    },
  ];

  return (
    <div className="rounded-md border">
      <div className="border-b px-3 py-2 text-sm font-medium">市场判断</div>
      <div className="divide-y">
        {rows.map((item) => {
          const isUnknown = !item.value || item.value === "unknown";
          return (
            <div
              key={item.key}
              data-market-judgment-row
              className="grid grid-cols-[5rem_minmax(0,1fr)] items-start gap-x-2 gap-y-1.5 px-3 py-2 text-sm sm:grid-cols-[5rem_8rem_minmax(0,1fr)] sm:gap-y-0"
            >
              <Badge variant="outline" className="w-fit justify-center">
                {item.label}
              </Badge>
              {isUnknown ? (
                <span className="text-xs leading-6 text-muted-foreground">-</span>
              ) : (
                <Badge className={`${item.className} w-fit gap-1.5`}>
                  <item.Icon className="h-3.5 w-3.5" />
                  {translate(item.value || "unknown")}
                </Badge>
              )}
              <div data-market-judgment-reason className="col-span-2 text-xs leading-relaxed text-muted-foreground sm:col-span-1">
                {item.reason || "未给出原因"}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
