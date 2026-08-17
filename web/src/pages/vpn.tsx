import {
  Ban,
  Cloud,
  Copy,
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
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notify } from "@/lib/notify";
import {
  createVPNDevice,
  fetchVPNQRCode,
  fetchVPNSummary,
  publishVPNDevice,
  revokeVPNDevice,
  rotateVPNDevice,
  type VPNDevice,
  type VPNFormat,
  type VPNSummary,
} from "@/lib/vpn";

type QRTarget = {
  device: VPNDevice;
  format: VPNFormat;
};

export function VPNPage() {
  const [summary, setSummary] = useState<VPNSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [deviceName, setDeviceName] = useState("");
  const [profileCode, setProfileCode] = useState("");
  const [busyDeviceID, setBusyDeviceID] = useState<number | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<VPNDevice | null>(null);
  const [rotateTarget, setRotateTarget] = useState<VPNDevice | null>(null);
  const [qrTarget, setQRTarget] = useState<QRTarget | null>(null);

  useEffect(() => {
    void loadSummary();
  }, []);

  async function loadSummary() {
    setIsLoading(true);
    try {
      const data = await fetchVPNSummary();
      setSummary(data);
      if (!profileCode && data.profiles.length) {
        setProfileCode(data.profiles[0].code);
      }
    } catch (error) {
      notify.errorFrom(error, "VPN 订阅状态加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleCreate() {
    if (!deviceName.trim() || !profileCode) {
      notify.warning("请填写设备名称并选择 VPN 资源");
      return;
    }
    setBusyDeviceID(0);
    try {
      await createVPNDevice(deviceName.trim(), profileCode);
      notify.success("设备订阅已创建");
      setDeviceName("");
      setIsCreateOpen(false);
      await loadSummary();
    } catch (error) {
      notify.errorFrom(error, "创建设备订阅失败");
    } finally {
      setBusyDeviceID(null);
    }
  }

  async function handlePublish(device: VPNDevice) {
    await runDeviceAction(device, publishVPNDevice, "订阅配置已重新发布", "重新发布失败");
  }

  async function handleRotate() {
    if (!rotateTarget) {
      return;
    }
    const target = rotateTarget;
    setRotateTarget(null);
    await runDeviceAction(target, rotateVPNDevice, "订阅地址已轮换，旧地址已失效", "轮换订阅地址失败");
  }

  async function handleRevoke() {
    if (!revokeTarget) {
      return;
    }
    const target = revokeTarget;
    setRevokeTarget(null);
    await runDeviceAction(target, revokeVPNDevice, "设备订阅已撤销", "撤销设备订阅失败");
  }

  async function runDeviceAction(
    device: VPNDevice,
    action: (deviceID: number) => Promise<VPNDevice>,
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

  const activeCount = summary?.devices.filter((device) => device.status === "active").length ?? 0;
  const formatCount = useMemo(() => {
    const formats = new Set(summary?.profiles.flatMap((profile) => profile.formats.map((format) => format.code)) ?? []);
    return formats.size;
  }, [summary]);

  if (isLoading && !summary) {
    return <VPNPageSkeleton />;
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatusCard label="VPN 资源" value={`${summary?.profiles.length ?? 0}`} detail="本机私有目录" icon={Wifi} />
        <StatusCard label="有效设备" value={`${activeCount}`} detail={`共 ${summary?.devices.length ?? 0} 台`} icon={KeyRound} />
        <StatusCard
          label="直连订阅"
          value={summary?.distributor_configured ? "已配置" : "未配置"}
          detail={summary?.distributor_url || "等待公开地址"}
          icon={Cloud}
        />
        <StatusCard label="客户端格式" value={`${formatCount}`} detail="按现有源文件生成" icon={QrCode} />
      </div>

      {!summary?.distributor_configured ? (
        <Alert>
          <Cloud className="h-4 w-4" />
          <AlertTitle>Go 直连订阅地址尚未配置</AlertTitle>
          <AlertDescription>现在可以先创建设备草稿；配置无需代理即可访问的服务器地址后，再从设备菜单发布订阅。</AlertDescription>
        </Alert>
      ) : null}

      <SourceCard summary={summary} />

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>设备订阅</CardTitle>
            <CardDescription>每台设备使用独立地址，可单独轮换或撤销。</CardDescription>
          </div>
          <Button
            type="button"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            disabled={!summary?.profiles.length}
          >
            <Plus className="h-4 w-4" />
            添加设备
          </Button>
        </CardHeader>
        <CardContent>
          <DeviceTable
            summary={summary}
            busyDeviceID={busyDeviceID}
            onCopy={copySubscription}
            onQR={setQRTarget}
            onPublish={handlePublish}
            onRotate={setRotateTarget}
            onRevoke={setRevokeTarget}
          />
        </CardContent>
      </Card>

      <CreateDeviceDialog
        open={isCreateOpen}
        name={deviceName}
        profileCode={profileCode}
        profiles={summary?.profiles ?? []}
        isSaving={busyDeviceID === 0}
        onOpenChange={setIsCreateOpen}
        onNameChange={setDeviceName}
        onProfileChange={setProfileCode}
        onSave={handleCreate}
      />

      <QRCodeDialog target={qrTarget} onOpenChange={(open) => !open && setQRTarget(null)} />

      <AlertDialog open={Boolean(rotateTarget)} onOpenChange={(open) => !open && setRotateTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>轮换订阅地址</AlertDialogTitle>
            <AlertDialogDescription>
              「{rotateTarget?.name}」的旧地址会立即失效，需要在对应终端重新填写或扫码。
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
            <AlertDialogTitle>撤销设备订阅</AlertDialogTitle>
            <AlertDialogDescription>
              「{revokeTarget?.name}」将无法继续更新订阅，设备记录会保留用于审计。
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

function SourceCard({ summary }: { summary: VPNSummary | null }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>私有资源</CardTitle>
        <CardDescription>只读取 storage/private/vpn，节点地址和密钥不会写入数据库。</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>资源</TableHead>
              <TableHead>可用客户端</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(summary?.profiles ?? []).map((profile) => (
              <TableRow key={profile.code}>
                <TableCell>
                  <div className="font-medium">{profile.name}</div>
                  <div className="font-mono text-xs text-muted-foreground">{profile.code}</div>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1.5">
                    {profile.formats.map((format) => <Badge key={format.code} variant="secondary">{format.name}</Badge>)}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {!summary?.profiles.length ? (
              <TableRow>
                <TableCell colSpan={2}>
                  <Empty className="border-0 py-5">
                    <EmptyHeader>
                      <EmptyTitle>没有检测到 VPN 资源</EmptyTitle>
                      <EmptyDescription>请检查私有目录中的文件命名和读取权限。</EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function DeviceTable({
  summary,
  busyDeviceID,
  onCopy,
  onQR,
  onPublish,
  onRotate,
  onRevoke,
}: {
  summary: VPNSummary | null;
  busyDeviceID: number | null;
  onCopy: (url: string) => void;
  onQR: (target: QRTarget) => void;
  onPublish: (device: VPNDevice) => void;
  onRotate: (device: VPNDevice) => void;
  onRevoke: (device: VPNDevice) => void;
}) {
  const profileMap = new Map((summary?.profiles ?? []).map((profile) => [profile.code, profile]));
  return (
    <div className="overflow-x-auto">
      <Table className="min-w-[760px]">
        <TableHeader>
          <TableRow>
            <TableHead>设备</TableHead>
            <TableHead>资源</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>客户端订阅</TableHead>
            <TableHead>最近发布</TableHead>
            <TableHead className="w-12 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(summary?.devices ?? []).map((device) => {
            const profile = profileMap.get(device.profile_code);
            const formats = profile?.formats.filter((format) => device.subscriptions[format.code]) ?? [];
            const isBusy = busyDeviceID === device.id;
            return (
              <TableRow key={device.id}>
                <TableCell>
                  <div className="font-medium">{device.name}</div>
                  <div className="text-xs text-muted-foreground">密钥版本 {device.token_version}</div>
                </TableCell>
                <TableCell>{profile?.name || device.profile_code}</TableCell>
                <TableCell>
                  <DeviceStatusBadge device={device} />
                  {device.last_error ? <div className="mt-1 max-w-56 text-xs text-destructive">{device.last_error}</div> : null}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {formats.map((format) => (
                      <div key={format.code} className="inline-flex items-center rounded-md border">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 rounded-r-none px-2"
                          onClick={() => onCopy(device.subscriptions[format.code])}
                        >
                          <Copy className="h-3.5 w-3.5" />
                          {format.name}
                        </Button>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 rounded-l-none border-l"
                              onClick={() => onQR({ device, format })}
                              aria-label={`${format.name} 二维码`}
                            >
                              <QrCode className="h-3.5 w-3.5" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>扫码订阅</TooltipContent>
                        </Tooltip>
                      </div>
                    ))}
                    {!formats.length ? <span className="text-sm text-muted-foreground">-</span> : null}
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">{formatTime(device.published_at)}</TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="icon" disabled={isBusy} aria-label="设备操作">
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
                        撤销设备
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            );
          })}
          {!summary?.devices.length ? (
            <TableRow>
              <TableCell colSpan={6}>
                <Empty className="border-0 py-8">
                  <EmptyHeader>
                    <EmptyTitle>还没有设备订阅</EmptyTitle>
                    <EmptyDescription>添加设备后会生成独立的 HTTPS 地址和二维码。</EmptyDescription>
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

function CreateDeviceDialog({
  open,
  name,
  profileCode,
  profiles,
  isSaving,
  onOpenChange,
  onNameChange,
  onProfileChange,
  onSave,
}: {
  open: boolean;
  name: string;
  profileCode: string;
  profiles: VPNSummary["profiles"];
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onNameChange: (name: string) => void;
  onProfileChange: (code: string) => void;
  onSave: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>添加订阅设备</DialogTitle>
          <DialogDescription>设备使用独立地址，丢失或弃用时可单独撤销。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <Field>
            <FieldLabel>设备名称</FieldLabel>
            <Input value={name} onChange={(event) => onNameChange(event.target.value)} placeholder="例如 OnePlus 13" maxLength={60} />
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
            创建设备
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
    fetchVPNQRCode(target.device.id, target.format.code)
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
          <DialogTitle>{target?.device.name}</DialogTitle>
          <DialogDescription>{target?.format.name} 订阅二维码</DialogDescription>
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

function DeviceStatusBadge({ device }: { device: VPNDevice }) {
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

async function copySubscription(value: string) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(value);
    } else {
      const textarea = document.createElement("textarea");
      textarea.value = value;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      textarea.remove();
    }
    notify.success("订阅地址已复制");
  } catch {
    notify.error("复制失败，请手动选择订阅地址");
  }
}
