import { Cloud, KeyRound, Plus, Wifi } from "lucide-react";
import { useEffect, useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { copyTextToClipboard } from "@/lib/clipboard";
import { notify } from "@/lib/notify";
import {
  createVPNUserSubscription,
  fetchVPNDistributionSummary,
  publishVPNUserSubscription,
  revokeVPNUserSubscription,
  rotateVPNUserSubscription,
  type VPNUserSubscription,
  type VPNSummary,
} from "@/lib/vpn";
import { UserSubscriptionTable } from "./subscription-table";
import { CreateUserDialog } from "./create-user-dialog";
import { StatusCard, VPNPageSkeleton } from "./presentation";
import { CommonRoutingCard } from "./common-routing-card";

export function VPNDistributionPage() {
  const [summary, setSummary] = useState<VPNSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [userID, setUserID] = useState("");
  const [profileCode, setProfileCode] = useState("");
  const [busySubscriptionID, setBusySubscriptionID] = useState<number | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<VPNUserSubscription | null>(null);
  const [rotateTarget, setRotateTarget] = useState<VPNUserSubscription | null>(null);

  useEffect(() => {
    void loadSummary();
  }, []);

  async function loadSummary() {
    setIsLoading(true);
    try {
      const data = await fetchVPNDistributionSummary();
      setSummary(data);
      if (!profileCode && data.profiles.length) {
        setProfileCode(data.profiles[0].code);
      }
      if (!userID) {
        const availableUser = data.users.find((user) => !user.has_subscription);
        setUserID(availableUser ? String(availableUser.id) : "");
      }
    } catch (error) {
      notify.errorFrom(error, "VPN 订阅状态加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleCreate() {
    const selectedUserID = Number(userID);
    if (!Number.isInteger(selectedUserID) || selectedUserID <= 0 || !profileCode) {
      notify.warning("请选择登录用户和 VPN 资源");
      return;
    }
    setBusySubscriptionID(0);
    try {
      await createVPNUserSubscription(selectedUserID, profileCode);
      notify.success("用户订阅已开通");
      setUserID("");
      setIsCreateOpen(false);
      await loadSummary();
    } catch (error) {
      notify.errorFrom(error, "开通用户订阅失败");
    } finally {
      setBusySubscriptionID(null);
    }
  }

  async function handleCopyCommonRouting() {
    const body = summary?.common_routing?.body?.trim() ?? "";
    if (!body) {
      notify.warning("暂无公共规则配置");
      return;
    }
    const success = await copyTextToClipboard(body);
    if (!success) {
      notify.error("复制失败", "当前浏览器不允许写入剪贴板。");
      return;
    }
    notify.success("公共规则已复制");
  }

  async function handlePublish(subscription: VPNUserSubscription) {
    await runSubscriptionAction(subscription, publishVPNUserSubscription, "订阅配置已重新发布", "重新发布失败");
  }

  async function handleRotate() {
    if (!rotateTarget) {
      return;
    }
    const target = rotateTarget;
    setRotateTarget(null);
    await runSubscriptionAction(target, rotateVPNUserSubscription, "订阅地址已轮换，旧地址已失效", "轮换订阅地址失败");
  }

  async function handleRevoke() {
    if (!revokeTarget) {
      return;
    }
    const target = revokeTarget;
    setRevokeTarget(null);
    await runSubscriptionAction(target, revokeVPNUserSubscription, "用户订阅已撤销", "撤销用户订阅失败");
  }

  async function runSubscriptionAction(
    subscription: VPNUserSubscription,
    action: (subscriptionID: number) => Promise<VPNUserSubscription>,
    successMessage: string,
    failureMessage: string,
  ) {
    setBusySubscriptionID(subscription.id);
    try {
      await action(subscription.id);
      notify.success(successMessage);
      await loadSummary();
    } catch (error) {
      notify.errorFrom(error, failureMessage);
    } finally {
      setBusySubscriptionID(null);
    }
  }

  const activeCount = summary?.user_subscriptions.filter((subscription) => subscription.status === "active").length ?? 0;
  if (isLoading && !summary) {
    return <VPNPageSkeleton />;
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatusCard label="VPN 资源" value={`${summary?.profiles.length ?? 0}`} detail={summary?.can_manage ? "本机私有目录" : "当前账号可用"} icon={Wifi} />
        <StatusCard label="登录用户" value={`${summary?.users.length ?? 0}`} detail="可分配账号" icon={KeyRound} />
        <StatusCard label="已分配" value={`${activeCount}`} detail={`共 ${summary?.user_subscriptions.length ?? 0} 条记录`} icon={KeyRound} />
        <StatusCard
          label="直连订阅"
          value={summary?.distributor_configured ? "已配置" : "未配置"}
          detail={summary?.distributor_url || "等待公开地址"}
          icon={Cloud}
        />
      </div>

      {!summary?.distributor_configured ? (
        <Alert>
          <Cloud className="h-4 w-4" />
          <AlertTitle>Go 直连订阅地址尚未配置</AlertTitle>
          <AlertDescription>现在可以先创建用户订阅草稿；配置无需代理即可访问的服务器地址后，再从菜单发布订阅。</AlertDescription>
        </Alert>
      ) : null}

      {summary?.can_manage ? (
        <CommonRoutingCard routing={summary.common_routing} onCopy={() => void handleCopyCommonRouting()} />
      ) : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>用户分配</CardTitle>
            <CardDescription>同一登录用户可以分配多套 VPN 资源，每套资源可在多台终端中共用。</CardDescription>
          </div>
          {summary?.can_manage ? <Button
            type="button"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            disabled={!summary?.profiles.length}
          >
            <Plus className="h-4 w-4" />
            开通用户
          </Button> : null}
        </CardHeader>
        <CardContent>
          <UserSubscriptionTable
            summary={summary}
            busySubscriptionID={busySubscriptionID}
            onPublish={handlePublish}
            onRotate={setRotateTarget}
            onRevoke={setRevokeTarget}
          />
        </CardContent>
      </Card>

      <CreateUserDialog
        open={isCreateOpen}
        userID={userID}
        profileCode={profileCode}
        users={summary?.users ?? []}
        profiles={summary?.profiles ?? []}
        isSaving={busySubscriptionID === 0}
        onOpenChange={setIsCreateOpen}
        onUserChange={setUserID}
        onProfileChange={setProfileCode}
        onSave={handleCreate}
      />

      <AlertDialog open={Boolean(rotateTarget)} onOpenChange={(open) => !open && setRotateTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>轮换订阅地址</AlertDialogTitle>
            <AlertDialogDescription>
              「{rotateTarget?.username}」的旧地址会立即失效，需要在对应终端重新填写或扫码。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleRotate()}>确认轮换</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={Boolean(revokeTarget)} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>撤销用户订阅</AlertDialogTitle>
            <AlertDialogDescription>
              「{revokeTarget?.username}」将无法继续更新订阅，记录会保留用于审计。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={() => void handleRevoke()}>
              确认撤销
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
