import type { Insight } from "./types";

export const sensitiveMask = "••••••";

export function toNumber(value: string | number | null | undefined) {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : 0;
}

export function formatMoney(value: string | number | null | undefined) {
  return toNumber(value).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

export function formatCompactMoney(value: string | number | null | undefined) {
  const numberValue = toNumber(value);
  if (Math.abs(numberValue) >= 10000) {
    return `${(numberValue / 10000).toFixed(1)}万`;
  }
  return numberValue.toLocaleString("zh-CN", { maximumFractionDigits: 0 });
}

export function formatSignedMoney(value: string | number | null | undefined) {
  const numberValue = toNumber(value);
  const prefix = numberValue > 0 ? "+" : "";
  return `${prefix}${formatMoney(numberValue)}`;
}

export function formatPercent(value: string | number | null | undefined) {
  return `${toNumber(value).toFixed(2)}%`;
}

export function changeTone(value: string | number | null | undefined) {
  const numberValue = toNumber(value);
  if (numberValue > 0) {
    return "text-emerald-700";
  }
  if (numberValue < 0) {
    return "text-red-700";
  }
  return "text-foreground";
}

export function maskSensitive(isSensitiveMasked: boolean, value: string) {
  return isSensitiveMasked ? sensitiveMask : value;
}

export function maskAccountIdentity(isSensitiveMasked: boolean, value: string) {
  return isSensitiveMasked ? "账户 " + sensitiveMask : value;
}

export function maskAccountText(isSensitiveMasked: boolean, value: string) {
  if (!isSensitiveMasked) {
    return value;
  }
  return value.replace(/东莞证券/g, "券商").replace(/邓子豪/g, "账户A").replace(/吴素尤/g, "账户B");
}

export function sensitiveTone(isSensitiveMasked: boolean, value: string | number | null | undefined) {
  return isSensitiveMasked ? "text-foreground" : changeTone(value);
}

export function getInsightValue(item: Insight, isSensitiveMasked: boolean) {
  return isSensitiveMasked && item.title.includes("资产") ? sensitiveMask : item.value;
}

export function getInsightDetail(item: Insight, isSensitiveMasked: boolean) {
  if (!isSensitiveMasked) {
    return item.detail;
  }
  if (item.title.includes("现金")) {
    return "最新可用资金 " + sensitiveMask;
  }
  return item.detail;
}
