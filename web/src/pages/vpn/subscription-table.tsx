import { Ban, Ellipsis, RefreshCw, RotateCw, UploadCloud } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { type VPNUserSubscription, type VPNSummary } from "@/lib/vpn";
import { SubscriptionStatusBadge } from "./presentation";

export function UserSubscriptionTable({
  summary,
  busySubscriptionID,
  onPublish,
  onRotate,
  onRevoke,
}: {
  summary: VPNSummary | null;
  busySubscriptionID: number | null;
  onPublish: (subscription: VPNUserSubscription) => void;
  onRotate: (subscription: VPNUserSubscription) => void;
  onRevoke: (subscription: VPNUserSubscription) => void;
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
          {(summary?.user_subscriptions ?? []).map((subscription) => {
            const profile = profileMap.get(subscription.profile_code);
            const isBusy = busySubscriptionID === subscription.id;
            return (
              <TableRow key={subscription.id}>
                <TableCell>
                  <div className="font-medium">{subscription.username}</div>
                  <div className="text-xs text-muted-foreground">密钥版本 {subscription.token_version}</div>
                </TableCell>
                <TableCell>{profile?.name || subscription.profile_code}</TableCell>
                <TableCell>
                  <SubscriptionStatusBadge subscription={subscription} />
                  {subscription.last_error ? <div className="mt-1 max-w-56 text-xs text-destructive">{subscription.last_error}</div> : null}
                </TableCell>
                <TableCell className="text-muted-foreground">{formatTime(subscription.published_at)}</TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="icon" disabled={isBusy} aria-label="用户订阅操作">
                        {isBusy ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Ellipsis className="h-4 w-4" />}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => onPublish(subscription)} disabled={subscription.status === "revoked"}>
                        <UploadCloud className="h-4 w-4" />
                        重新发布
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => onRotate(subscription)} disabled={subscription.status === "revoked"}>
                        <RotateCw className="h-4 w-4" />
                        轮换地址
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => onRevoke(subscription)} disabled={subscription.status === "revoked"}>
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

function formatTime(value?: string | null) {
  return value ? value.slice(0, 16) : "-";
}
