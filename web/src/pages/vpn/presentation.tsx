import { Wifi } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { type VPNUserSubscription } from "@/lib/vpn";

export function StatusCard({ label, value, detail, icon: Icon }: { label: string; value: string; detail: string; icon: typeof Wifi }) {
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

export function SubscriptionStatusBadge({ subscription }: { subscription: VPNUserSubscription }) {
  if (subscription.status === "active") {
    return <Badge variant="success">有效</Badge>;
  }
  if (subscription.status === "error") {
    return <Badge variant="danger">发布失败</Badge>;
  }
  if (subscription.status === "revoked") {
    return <Badge variant="secondary">已撤销</Badge>;
  }
  return <Badge variant="outline">待发布</Badge>;
}

export function VPNPageSkeleton() {
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
