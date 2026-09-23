import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { VPNSummary } from "@/lib/vpn";

// 两个 VPN 页面读取同一份规则；只有管理页传入复制操作，不提供编辑入口。
export function CommonRoutingCard({ routing, onCopy }: {
  routing?: VPNSummary["common_routing"];
  onCopy?: () => void;
}) {
  return (
    <Card>
      <CardHeader className={onCopy ? "flex flex-row items-start justify-between gap-3" : undefined}>
        <div>
          <CardTitle>公共分流规则</CardTitle>
          <CardDescription>{onCopy
            ? "只读查看所有 VPN 订阅共用的最新规则；规则文件由 AI 更新并随部署生效。"
            : "所有 VPN 订阅共用的域名与网络分流规则；此处仅供查看。"}</CardDescription>
        </div>
        {onCopy ? <Button type="button" variant="outline" size="sm" onClick={onCopy}>
          <Copy className="h-4 w-4" />
          复制规则
        </Button> : null}
      </CardHeader>
      <CardContent>
        <div className="mb-2 text-xs text-muted-foreground">{routing?.filename || "common-routing.json"}</div>
        <pre className="max-h-72 overflow-auto rounded-lg border bg-muted/30 p-4 text-xs leading-5 whitespace-pre-wrap">
          {routing?.body || "暂无公共规则配置"}
        </pre>
      </CardContent>
    </Card>
  );
}
