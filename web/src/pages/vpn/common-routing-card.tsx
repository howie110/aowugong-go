import { describeRouting } from "./routing-display";
import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { VPNSummary } from "@/lib/vpn";

// 两个 VPN 页面读取同一份规则；只有管理页传入复制操作，不提供编辑入口。
export function CommonRoutingCard({ routing, onCopy }: {
  routing?: VPNSummary["common_routing"];
  onCopy?: () => void;
}) {
  let sections: ReturnType<typeof describeRouting> = [];
  let invalid = false;
  if (routing?.body?.trim()) {
    try { sections = describeRouting(routing.body); } catch { invalid = true; }
  }
  return (
    <Card>
      <CardHeader className={onCopy ? "flex flex-row items-start justify-between gap-3" : undefined}>
        <div>
          <CardTitle>公共分流规则</CardTitle>
          <CardDescription>只读 · 按顺序匹配</CardDescription>
        </div>
        {onCopy ? <Button type="button" variant="outline" size="sm" onClick={onCopy}>
          <Copy className="h-4 w-4" />
          复制规则
        </Button> : null}
      </CardHeader>
      <CardContent>
        {!routing?.body?.trim() ? <p className="text-sm text-muted-foreground">暂无公共规则配置</p> : invalid ? (
          <div className="space-y-3">
            <p className="text-sm text-destructive">规则格式暂无法逐条解释，以下保留完整原文供检查。</p>
            <pre className="whitespace-pre-wrap break-all rounded-lg border p-4 text-xs leading-6">{routing.body}</pre>
          </div>
        ) : (
          <div className="space-y-3">

            {sections?.length === 0 ? <p className="text-sm text-muted-foreground">当前规则列表为空</p> : null}
            {sections?.map((section, index) => (
              <section key={index} className="min-w-0">
                <h3 className="mb-1 font-mono text-xs text-muted-foreground sm:text-sm"># {section.title}</h3>
                <ol className="font-mono text-xs leading-6 sm:text-sm">
                  {section.rows.map(row => (
                    <li key={row.order} className="min-w-0">
                      <code className="whitespace-pre-wrap break-all">{row.line}</code>
                    </li>
                  ))}
                </ol>
              </section>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
