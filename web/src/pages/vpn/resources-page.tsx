import { Copy, QrCode } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { copyTextToClipboard } from "@/lib/clipboard";
import { notify } from "@/lib/notify";
import {
  fetchVPNResourceSummary,
  type VPNUserSubscription,
  type VPNFormat,
  type VPNSummary,
} from "@/lib/vpn";
import { QRCodeDialog, type QRTarget } from "./qr-code-dialog";
import { SubscriptionStatusBadge, VPNPageSkeleton } from "./presentation";
import { CommonRoutingCard } from "./common-routing-card";

// VPNResourcesPage 展示当前登录用户获配的 VPN 资源和客户端二维码。
// 输入：无。
// 输出：返回只包含当前用户资源的页面。
// 副作用：请求 Go API，点击客户端时读取二维码。
export function VPNResourcesPage() {
  const [summary, setSummary] = useState<VPNSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [qrTarget, setQRTarget] = useState<QRTarget | null>(null);

  useEffect(() => {
    // 1. 页面加载时读取服务端按当前用户过滤的资源。
    fetchVPNResourceSummary()
      .then(setSummary)
      .catch((error) => notify.errorFrom(error, "VPN 资源加载失败"))
      .finally(() => setIsLoading(false));
  }, []);

  const profileMap = useMemo(
    () => new Map((summary?.profiles ?? []).map((profile) => [profile.code, profile])),
    [summary],
  );

  if (isLoading && !summary) {
    return <VPNPageSkeleton />;
  }

  const subscriptions = summary?.user_subscriptions ?? [];

  async function handleCopySubscription(subscription: VPNUserSubscription, format: VPNFormat) {
    const subscriptionURL = subscription.subscriptions[format.code];
    if (!subscriptionURL) {
      notify.warning("当前订阅链接不可用", "请联系管理员重新发布资源。");
      return;
    }
    const success = await copyTextToClipboard(subscriptionURL);
    if (!success) {
      notify.error("复制失败", "当前浏览器不允许写入剪贴板。");
      return;
    }
    notify.success("订阅链接已复制", format.name);
  }

  return (
    <div className="space-y-4">
      <CommonRoutingCard routing={summary?.common_routing} />

      {subscriptions.map((subscription) => {
        const profile = profileMap.get(subscription.profile_code);
        const formats = profile?.formats.filter((format) => subscription.subscriptions[format.code]) ?? [];
        return (
          <Card key={subscription.id}>
            <CardHeader className="flex flex-row items-start justify-between gap-3">
              <div>
                <CardTitle>{profile?.name || subscription.profile_code}</CardTitle>
                <CardDescription>可扫码导入，也可复制订阅链接到客户端；手机和电脑可共用当前账号的资源。</CardDescription>
              </div>
              <SubscriptionStatusBadge subscription={subscription} />
            </CardHeader>
            <CardContent>
              {subscription.last_error ? (
                <Alert className="mb-4" variant="destructive">
                  <AlertTitle>资源暂不可用</AlertTitle>
                  <AlertDescription>{subscription.last_error}</AlertDescription>
                </Alert>
              ) : null}
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                {formats.map((format) => (
                  <div key={format.code} className="rounded-lg border p-3">
                    <div className="font-medium">{format.name}</div>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setQRTarget({ subscription, format, profileName: profile?.name || subscription.profile_code })}
                      >
                        <QrCode className="h-4 w-4" />
                        扫码配置
                      </Button>
                      <Button type="button" variant="outline" size="sm" onClick={() => void handleCopySubscription(subscription, format)}>
                        <Copy className="h-4 w-4" />
                        复制链接
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
              {!formats.length ? (
                <Empty className="border-0 py-8">
                  <EmptyHeader>
                    <EmptyTitle>当前资源不可扫码</EmptyTitle>
                    <EmptyDescription>请联系管理员检查发布状态或重新分配。</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : null}
            </CardContent>
          </Card>
        );
      })}

      {!subscriptions.length ? (
        <Card>
          <CardContent>
            <Empty className="border-0 py-12">
              <EmptyHeader>
                <EmptyTitle>尚未分配 VPN 资源</EmptyTitle>
                <EmptyDescription>管理员完成分配后，资源和二维码会显示在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : null}

      <QRCodeDialog target={qrTarget} onOpenChange={(open) => !open && setQRTarget(null)} />
    </div>
  );
}
