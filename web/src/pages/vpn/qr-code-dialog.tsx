import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { notify } from "@/lib/notify";
import { fetchVPNQRCode, type VPNUserSubscription, type VPNFormat } from "@/lib/vpn";

export type QRTarget = {
  subscription: VPNUserSubscription;
  format: VPNFormat;
  profileName: string;
};

export function QRCodeDialog({ target, onOpenChange }: { target: QRTarget | null; onOpenChange: (open: boolean) => void }) {
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
