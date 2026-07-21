import { LoaderCircle } from "lucide-react";

import { cn } from "@/lib/utils";

/** Spinner 展示统一的旋转加载图标，无副作用。 */
function Spinner({ className, ...props }: React.ComponentProps<typeof LoaderCircle>) {
  // 1. 使用 Lucide 图标和统一动画渲染加载状态。
  return <LoaderCircle role="status" aria-label="加载中" className={cn("h-4 w-4 animate-spin", className)} {...props} />;
}

export { Spinner };
