import { Copy, Eye, EyeOff, KeyRound, Link, Terminal } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { type NotificationExternalConfig, fetchNotificationExternalConfig } from "@/lib/finance";
import { notify } from "@/lib/notify";

const tokenPlaceholder = "<EXTERNAL_TOKEN>";

/** NotificationExternal 展示外部服务调用通知 API 的固定 key、示例和 MCP 工具说明。 */
export function NotificationExternal() {
  const [config, setConfig] = useState<NotificationExternalConfig | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showKey, setShowKey] = useState(false);

  useEffect(() => {
    // 1. 进入页面时读取 external 项目的固定调用配置。
    let cancelled = false;
    fetchNotificationExternalConfig()
      .then((result) => {
        if (!cancelled) {
          setConfig(result);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          notify.errorFrom(error, "读取外部调用配置失败", "加载失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const endpoint = config ? `${window.location.origin}${config.endpoint}` : "";
  const token = config?.token || "";
  const curlExample = buildCurl(endpoint, tokenPlaceholder);
  const mcpExample = JSON.stringify(buildMCP(endpoint, tokenPlaceholder), null, 2);
  const promptExample = buildPrompt(endpoint, tokenPlaceholder);

  /** copy 把文本写入剪贴板并反馈结果。 */
  async function copy(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text);
      notify.success("已复制", label);
    } catch (error) {
      notify.errorFrom(error, "复制失败");
    }
  }

  if (isLoading) {
    return <Skeleton className="h-64" />;
  }

  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle>外部调用 / MCP</CardTitle>
        <CardDescription>
          其他服务或 AI 可以用固定 key 调用这个接口发送纯文本微信通知，不支持文件或附件。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!config?.configured ? (
          <Alert>
            <AlertTitle>尚未配置 external 项目 Token</AlertTitle>
            <AlertDescription>
              在服务器 `AOWUGONG_NOTIFICATION_TOKENS` 中加入 `external` 项目 Token 并重启应用后，
              这里会显示可复制的 key。不要复用其他项目的 Token。
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="space-y-2">
          <div className="text-sm font-medium">请求地址</div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input readOnly value={endpoint} className="font-mono text-xs" />
            <Button type="button" variant="outline" onClick={() => void copy(endpoint, "请求地址")}>
              <Link className="h-4 w-4" />
              复制
            </Button>
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm font-medium">
            <KeyRound className="h-4 w-4" />
            固定 Key
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input readOnly type={showKey ? "text" : "password"} value={token} className="font-mono text-xs" />
            <Button type="button" variant="outline" onClick={() => setShowKey((value) => !value)}>
              {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              {showKey ? "隐藏" : "显示"}
            </Button>
            <Button type="button" variant="outline" disabled={!token} onClick={() => void copy(token, "固定 Key")}>
              <Copy className="h-4 w-4" />
              复制
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            此 key 等同于发送权限，只用于纯文本通知，不要发到公开渠道。所有调用共用 `external` 来源，
            `request_id` 建议带上服务名前缀避免冲突。
          </p>
        </div>

        <CodeBlock
          title="HTTP 调用示例"
          icon={<Terminal className="h-4 w-4" />}
          code={curlExample}
          onCopy={() => void copy(buildCurl(endpoint, token || tokenPlaceholder), "HTTP 调用示例")}
        />

        <CodeBlock
          title="MCP 工具描述（可直接复制给 AI/Agent）"
          icon={<Copy className="h-4 w-4" />}
          code={mcpExample}
          onCopy={() => void copy(JSON.stringify(buildMCP(endpoint, token || tokenPlaceholder), null, 2), "MCP 工具描述")}
        />

        <CodeBlock
          title="给 AI 的使用说明"
          icon={<Copy className="h-4 w-4" />}
          code={promptExample}
          onCopy={() => void copy(buildPrompt(endpoint, token || tokenPlaceholder), "使用说明")}
        />

        <div className="rounded-md border p-3 text-sm text-muted-foreground">
          <div className="font-medium text-foreground">调用规则</div>
          <ul className="mt-2 list-disc space-y-1 pl-5">
            <li>只接受 `request_id`、`title`、`content` 三个字段，不支持文件或附件。</li>
            <li>标题单行；完整消息最多 2048 UTF-8 字节；请求最多 16 KiB。</li>
            <li>同一来源、同一 `request_id`、相同内容幂等；相同编号不同内容返回 409。</li>
            <li>全部调用共享每分钟 10 次新发送额度，返回 429 时按 `Retry-After` 等待。</li>
            <li>鉴权头为 `Authorization: Bearer &lt;固定 Key&gt;`，不能用网页登录 Token 代替。</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  );
}

/** CodeBlock 展示只读示例和复制按钮。 */
function CodeBlock({ title, icon, code, onCopy }: { title: string; icon: ReactNode; code: string; onCopy: () => void }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium">{icon}{title}</div>
        <Button type="button" variant="outline" size="sm" onClick={onCopy}>
          <Copy className="h-4 w-4" />
          复制
        </Button>
      </div>
      <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-4 text-xs">
        {code}
      </pre>
    </div>
  );
}

/** buildCurl 生成可直接执行的通知调用示例。 */
function buildCurl(endpoint: string, token: string) {
  return `curl -X POST "${endpoint}" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{"request_id":"my-service-event-001","title":"部署完成","content":"服务已更新"}'`;
}

/** buildMCP 生成给 AI/Agent 的工具描述；只支持文本。 */
function buildMCP(endpoint: string, token: string) {
  return {
    name: "send_notification",
    description: "Send a plain-text WeChat notification. Text only; no files or attachments.",
    endpoint,
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    inputSchema: {
      type: "object",
      properties: {
        request_id: {
          type: "string",
          description: "Unique idempotency key, e.g. service-name-event-id. Same id with different content returns 409.",
        },
        title: {
          type: "string",
          description: "Single-line notification title.",
        },
        content: {
          type: "string",
          description: "Plain text notification body. The final message is limited to 2048 UTF-8 bytes.",
        },
      },
      required: ["request_id", "title", "content"],
    },
  };
}

/** buildPrompt 生成一段可直接喂给 AI 的调用说明。 */
function buildPrompt(endpoint: string, token: string) {
  return [
    "你可以调用 send_notification 发送微信通知。",
    `接口：POST ${endpoint}`,
    `鉴权：Authorization: Bearer ${token}`,
    "请求体：{\"request_id\":\"唯一编号\",\"title\":\"单行标题\",\"content\":\"纯文本正文\"}",
    "规则：只发文本，不发文件；request_id 用于幂等；正文最多 2048 UTF-8 字节；全局限流每分钟 10 次。",
  ].join("\n");
}
