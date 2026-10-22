import { useEffect, useState } from "react";
import { Copy, Smartphone } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { TrajectoryCredential } from "@/lib/trajectory";

export function GPSLoggerSetup({ credential, onClose }: {
  credential: TrajectoryCredential | null;
  onClose: () => void;
}) {
  const [notice, setNotice] = useState("");
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    setNotice("");
    setExpired(false);
    if (!credential) return;
    const timer = window.setTimeout(() => setExpired(true), 15 * 60 * 1000);
    return () => window.clearTimeout(timer);
  }, [credential]);
  const url = credential ? `${window.location.origin}${credential.profile_path}` : "";
  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setNotice("已复制，在 GPSLogger「从 URL」中粘贴即可。");
    } catch {
      setNotice("复制失败，请长按下方链接复制。");
    }
  }
  return (
    <Card>
      <CardHeader><CardTitle>连接 GPSLogger</CardTitle></CardHeader>
      <CardContent className="space-y-3 text-sm">
        {credential && !expired ? <>
          <p>「{credential.device.name}」配置已就绪，默认每 60 秒记录，保留本地 CSV 备份。</p>
          <div className="flex flex-wrap gap-2">
            <Button asChild><a href={`gpslogger://properties/${url}`}><Smartphone className="mr-2 h-4 w-4" />一键导入 GPSLogger</a></Button>
            <Button variant="outline" onClick={() => void copy()}><Copy className="mr-2 h-4 w-4" />复制配置链接</Button>
          </div>
          <p className="text-muted-foreground">请用手机浏览器打开。链接 15 分钟内有效，含设备凭证，请勿分享。</p>
          {notice && <p role="status">{notice}</p>}
          {notice.startsWith("复制失败") && <a className="block break-all underline" href={url} rel="noreferrer">{url}</a>}
        </> : <p className="text-muted-foreground">{expired ? "配置链接已过期。" : ""}添加设备后即可一键导入；已有设备可通过「更换密钥」重新生成配置，旧密钥会失效。</p>}
        {credential && <Button variant="ghost" onClick={onClose}>{expired ? "关闭" : "完成"}</Button>}
        <p className="text-muted-foreground">导入后允许后台定位、取消电池优化，再开始记录。</p>
      </CardContent>
    </Card>
  );
}
