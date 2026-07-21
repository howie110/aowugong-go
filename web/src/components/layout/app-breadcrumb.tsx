import { Check, Copy } from "lucide-react";
import { useState, type MouseEvent } from "react";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { type FinancePageKey, getFinancePagePath } from "@/lib/finance";
import { notify } from "@/lib/notify";
import { pageGroupLabelMap, pageLabelMap } from "./app-navigation";

export function AppBreadcrumb({ activePage, onNavigate }: { activePage: FinancePageKey; onNavigate: (pageKey: FinancePageKey) => void }) {
  const [isCopied, setIsCopied] = useState(false);
  const activeLabel = pageLabelMap[activePage];
  const activeGroupLabel = pageGroupLabelMap[activePage];
  const breadcrumbText = `工作台 > ${activeGroupLabel} > ${activeLabel}`;

  function handleBreadcrumbNavigate(event: MouseEvent<HTMLAnchorElement>, pageKey: FinancePageKey) {
    event.preventDefault();
    onNavigate(pageKey);
  }

  async function handleCopyLocation() {
    const success = await copyTextToClipboard(breadcrumbText);
    if (!success) {
      notify.error("复制失败", "当前浏览器不允许写入剪贴板。");
      return;
    }
    setIsCopied(true);
    notify.info("位置已复制", breadcrumbText);
    window.setTimeout(() => setIsCopied(false), 1200);
  }

  return (
    <div className="mt-1 flex min-w-0 items-center gap-1.5">
      <Breadcrumb className="min-w-0 select-text" title={breadcrumbText}>
        <BreadcrumbList className="flex-nowrap overflow-hidden select-text">
          <BreadcrumbItem>
            <BreadcrumbLink href={getFinancePagePath("overview")} onClick={(event) => handleBreadcrumbNavigate(event, "overview")}>
              工作台
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem className="hidden min-w-0 sm:block">
            <BreadcrumbPage className="truncate text-muted-foreground">{activeGroupLabel}</BreadcrumbPage>
          </BreadcrumbItem>
          <BreadcrumbSeparator className="hidden sm:block" />
          <BreadcrumbItem className="min-w-0">
            <BreadcrumbPage className="truncate">{activeLabel}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className="h-5 w-5 shrink-0" onClick={handleCopyLocation} aria-label="复制当前位置">
            {isCopied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>复制当前位置</TooltipContent>
      </Tooltip>
    </div>
  );
}

async function copyTextToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 旧浏览器或权限受限时，退回临时 textarea 复制。
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus({ preventScroll: true });
  textarea.select();
  textarea.setSelectionRange(0, text.length);
  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}
