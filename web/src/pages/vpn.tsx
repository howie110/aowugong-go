import {
  Ban,
  Cloud,
  Ellipsis,
  KeyRound,
  Plus,
  QrCode,
  RefreshCw,
  RotateCw,
  UploadCloud,
  Wifi,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";

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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { notify } from "@/lib/notify";
import {
  createVPNUserSubscription,
  fetchVPNQRCode,
  fetchVPNDistributionSummary,
  fetchVPNResourceSummary,
  publishVPNUserSubscription,
  revokeVPNUserSubscription,
  rotateVPNUserSubscription,
  type VPNUserSubscription,
  type VPNFormat,
  type VPNUserOption,
  type VPNSummary,
} from "@/lib/vpn";

type QRTarget = {
  subscription: VPNUserSubscription;
  format: VPNFormat;
  profileName: string;
};

export function VPNDistributionPage() {
  const [summary, setSummary] = useState<VPNSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [userID, setUserID] = useState("");
  const [profileCode, setProfileCode] = useState("");
  const [busyDeviceID, setBusyDeviceID] = useState<number | null>(null);
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
    setBusyDeviceID(0);
    try {
      await createVPNUserSubscription(selectedUserID, profileCode);
      notify.success("用户订阅已开通");
      setUserID("");
      setIsCreateOpen(false);
      await loadSummary();
    } catch (error) {
      notify.errorFrom(error, "开通用户订阅失败");
    } finally {
      setBusyDeviceID(null);
    }
  }

  async function handlePublish(device: VPNUserSubscription) {
    await runDeviceAction(device, publishVPNUserSubscription, "订阅配置已重新发布", "重新发布失败");
  }

  async function handleRotate() {
    if (!rotateTarget) {
      return;
    }
    const target = rotateTarget;
    setRotateTarget(null);
    await runDeviceAction(target, rotateVPNUserSubscription, "订阅地址已轮换，旧地址已失效", "轮换订阅地址失败");
  }

  async function handleRevoke() {
    if (!revokeTarget) {
      return;
    }
    const target = revokeTarget;
    setRevokeTarget(null);
    await runDeviceAction(target, revokeVPNUserSubscription, "用户订阅已撤销", "撤销用户订阅失败");
  }

  async function runDeviceAction(
    device: VPNUserSubscription,
    action: (deviceID: number) => Promise<VPNUserSubscription>,
    successMessage: string,
    failureMessage: string,
  ) {
    setBusyDeviceID(device.id);
    try {
      await action(device.id);
      notify.success(successMessage);
      await loadSummary();
    } catch (error) {
      notify.errorFrom(error, failureMessage);
    } finally {
      setBusyDeviceID(null);
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

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>用户分配</CardTitle>
            <CardDescription>一名登录用户分配一个 VPN 资源，可在多台终端中使用。</CardDescription>
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
            busyDeviceID={busyDeviceID}
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
        isSaving={busyDeviceID === 0}
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
  return (
    <div className="space-y-4">
      {subscriptions.map((subscription) => {
        const profile = profileMap.get(subscription.profile_code);
        const formats = profile?.formats.filter((format) => subscription.subscriptions[format.code]) ?? [];
        return (
          <Card key={subscription.id}>
            <CardHeader className="flex flex-row items-start justify-between gap-3">
              <div>
                <CardTitle>{profile?.name || subscription.profile_code}</CardTitle>
                <CardDescription>选择客户端后扫码导入，手机和电脑可共用当前账号的资源。</CardDescription>
              </div>
              <DeviceStatusBadge device={subscription} />
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
                  <Button
                    key={format.code}
                    type="button"
                    variant="outline"
                    className="h-auto min-h-20 justify-between px-4 py-3"
                    onClick={() => setQRTarget({ subscription, format, profileName: profile?.name || subscription.profile_code })}
                  >
                    <span className="text-left">
                      <span className="block font-medium">{format.name}</span>
                      <span className="mt-1 block text-xs font-normal text-muted-foreground">扫码配置</span>
                    </span>
                    <QrCode className="h-5 w-5" />
                  </Button>
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

function UserSubscriptionTable({
  summary,
  busyDeviceID,
  onPublish,
  onRotate,
  onRevoke,
}: {
  summary: VPNSummary | null;
  busyDeviceID: number | null;
  onPublish: (device: VPNUserSubscription) => void;
  onRotate: (device: VPNUserSubscription) => void;
  onRevoke: (device: VPNUserSubscription) => void;
}) {
  const profileMap = new Map((summary?.profiles ?? []).map((profile) => [profile.code, profile]));
  return (
    <div className="overflow-x-auto">
      <Table className="min-w-[620px]">
        <TableHeader>
          <TableRow>
            <TableHead>用户</TableHead>
            <TableHead>资源</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>最近发布</TableHead>
            <TableHead className="w-12 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(summary?.user_subscriptions ?? []).map((device) => {
            const profile = profileMap.get(device.profile_code);
            const isBusy = busyDeviceID === device.id;
            return (
              <TableRow key={device.id}>
                <TableCell>
                  <div className="font-medium">{device.username}</div>
                  <div className="text-xs text-muted-foreground">密钥版本 {device.token_version}</div>
                </TableCell>
                <TableCell>{profile?.name || device.profile_code}</TableCell>
                <TableCell>
                  <DeviceStatusBadge device={device} />
                  {device.last_error ? <div className="mt-1 max-w-56 text-xs text-destructive">{device.last_error}</div> : null}
                </TableCell>
                <TableCell className="text-muted-foreground">{formatTime(device.published_at)}</TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="icon" disabled={isBusy} aria-label="用户订阅操作">
                        {isBusy ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Ellipsis className="h-4 w-4" />}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => onPublish(device)} disabled={device.status === "revoked"}>
                        <UploadCloud className="h-4 w-4" />
                        重新发布
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => onRotate(device)} disabled={device.status === "revoked"}>
                        <RotateCw className="h-4 w-4" />
                        轮换地址
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => onRevoke(device)} disabled={device.status === "revoked"}>
                        <Ban className="h-4 w-4" />
                        撤销订阅
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            );
          })}
          {!summary?.user_subscriptions.length ? (
            <TableRow>
              <TableCell colSpan={5}>
                <Empty className="border-0 py-8">
                  <EmptyHeader>
                    <EmptyTitle>还没有用户分配</EmptyTitle>
                    <EmptyDescription>选择登录用户和 VPN 资源后即可完成分配。</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
  );
}

function CreateUserDialog({
  open,
  userID,
  profileCode,
  users,
  profiles,
  isSaving,
  onOpenChange,
  onUserChange,
  onProfileChange,
  onSave,
}: {
  open: boolean;
  userID: string;
  profileCode: string;
  users: VPNUserOption[];
  profiles: VPNSummary["profiles"];
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onUserChange: (userID: string) => void;
  onProfileChange: (code: string) => void;
  onSave: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>开通用户订阅</DialogTitle>
          <DialogDescription>一个登录用户使用一组地址，可在多台终端中共用。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <Field>
            <FieldLabel>登录用户</FieldLabel>
            <Select value={userID} onValueChange={onUserChange}>
              <SelectTrigger>
                <SelectValue placeholder="选择用户" />
              </SelectTrigger>
              <SelectContent>
                {users.filter((user) => !user.has_subscription).map((user) => (
                  <SelectItem key={user.id} value={String(user.id)}>{user.username} · {user.email}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>VPN 资源</FieldLabel>
            <Select value={profileCode} onValueChange={onProfileChange}>
              <SelectTrigger>
                <SelectValue placeholder="选择资源" />
              </SelectTrigger>
              <SelectContent>
                {profiles.map((profile) => <SelectItem key={profile.code} value={profile.code}>{profile.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </Field>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSaving}>取消</Button>
          <Button type="button" onClick={onSave} disabled={isSaving}>
            {isSaving ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            确认开通
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function QRCodeDialog({ target, onOpenChange }: { target: QRTarget | null; onOpenChange: (open: boolean) => void }) {
  const [imageURL, setImageURL] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (!target) {
      setImageURL("");
      return;
    }
    let objectURL = "";
    let cancelled = false;
    setImageURL("");
    setIsLoading(true);
    fetchVPNQRCode(target.subscription.id, target.format.code)
      .then((blob) => {
        if (cancelled) {
          return;
        }
        objectURL = URL.createObjectURL(blob);
        setImageURL(objectURL);
      })
      .catch((error) => notify.errorFrom(error, "二维码加载失败"))
      .finally(() => !cancelled && setIsLoading(false));
    return () => {
      cancelled = true;
      if (objectURL) {
        URL.revokeObjectURL(objectURL);
      }
    };
  }, [target]);

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{target?.profileName}</DialogTitle>
          <DialogDescription>{target?.format.name} 导入二维码</DialogDescription>
        </DialogHeader>
        <div className="flex aspect-square items-center justify-center rounded-md border bg-white p-4">
          {imageURL ? <img src={imageURL} alt="VPN 订阅二维码" className="h-full w-full object-contain" /> : null}
          {isLoading ? <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" /> : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function StatusCard({ label, value, detail, icon: Icon }: { label: string; value: string; detail: string; icon: typeof Wifi }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
        <p className="mt-1 truncate text-xs text-muted-foreground" title={detail}>{detail}</p>
      </CardContent>
    </Card>
  );
}

function DeviceStatusBadge({ device }: { device: VPNUserSubscription }) {
  if (device.status === "active") {
    return <Badge variant="success">有效</Badge>;
  }
  if (device.status === "error") {
    return <Badge variant="danger">发布失败</Badge>;
  }
  if (device.status === "revoked") {
    return <Badge variant="secondary">已撤销</Badge>;
  }
  return <Badge variant="outline">待发布</Badge>;
}

function VPNPageSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28" />)}
      </div>
      <Skeleton className="h-48" />
      <Skeleton className="h-72" />
    </div>
  );
}

function formatTime(value?: string | null) {
  return value ? value.slice(0, 16) : "-";
}
