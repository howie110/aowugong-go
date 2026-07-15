import { Frown, Meh, MoveRight, Smile, TrendingDown, TrendingUp } from "lucide-react";

import { Badge } from "@/components/ui/badge";

const labelMap: Record<string, string> = {
  unknown: "未知",
  very_optimistic: "极度乐观",
  optimistic: "乐观",
  neutral: "中性",
  pessimistic: "悲观",
  very_pessimistic: "极度悲观",
  up: "上涨",
  down: "下跌",
  range: "震荡",
};

export function MarketMoodBadge({ value }: { value?: string | null }) {
  if (!value || value === "unknown") {
    return <span className="text-xs text-muted-foreground">-</span>;
  }
  const Icon = getMarketMoodIcon(value);
  const label = translate(value || "unknown");
  return (
    <Badge className={getMarketBadgeIconClass(getMarketMoodClass(value))} title={label} aria-label={label}>
      <Icon className="h-3.5 w-3.5" />
    </Badge>
  );
}

export function MarketPredictionBadge({ value }: { value?: string | null }) {
  if (!value || value === "unknown") {
    return <span className="text-xs text-muted-foreground">-</span>;
  }
  const Icon = getMarketPredictionIcon(value);
  const label = translate(value || "unknown");
  return (
    <Badge className={getMarketBadgeIconClass(getMarketPredictionClass(value))} title={label} aria-label={label}>
      <Icon className="h-3.5 w-3.5" />
    </Badge>
  );
}

export function translate(value: string) {
  return labelMap[value] || value || "未知";
}

export function getDistributionBarClass(value: string) {
  if (value === "very_optimistic" || value === "optimistic" || value === "up") {
    return "bg-red-500";
  }
  if (value === "pessimistic" || value === "very_pessimistic" || value === "down") {
    return "bg-emerald-500";
  }
  if (value === "neutral" || value === "range") {
    return "bg-neutral-500";
  }
  return "bg-muted-foreground/40";
}

export function getMarketMoodIcon(value?: string | null) {
  if (value === "very_optimistic" || value === "optimistic") {
    return Smile;
  }
  if (value === "pessimistic" || value === "very_pessimistic") {
    return Frown;
  }
  return Meh;
}

export function getMarketPredictionIcon(value?: string | null) {
  if (value === "up") {
    return TrendingUp;
  }
  if (value === "down") {
    return TrendingDown;
  }
  return MoveRight;
}

export function getMarketMoodClass(value?: string | null) {
  if (value === "very_optimistic" || value === "optimistic") {
    return "border-transparent bg-red-50 text-red-700";
  }
  if (value === "pessimistic" || value === "very_pessimistic") {
    return "border-transparent bg-emerald-50 text-emerald-700";
  }
  if (value === "neutral") {
    return "border-transparent bg-neutral-100 text-neutral-900";
  }
  return "border-transparent bg-muted text-muted-foreground";
}

export function getMarketPredictionClass(value?: string | null) {
  if (value === "up") {
    return "border-transparent bg-red-50 text-red-700";
  }
  if (value === "down") {
    return "border-transparent bg-emerald-50 text-emerald-700";
  }
  if (value === "range") {
    return "border-transparent bg-neutral-100 text-neutral-900";
  }
  return "border-transparent bg-muted text-muted-foreground";
}


function getMarketBadgeIconClass(className: string) {
  return `${className} h-6 w-7 justify-center px-0`;
}
