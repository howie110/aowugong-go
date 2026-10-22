import { useState } from "react";
import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { gpsLoggerConfig, type TrajectoryCredential } from "@/lib/trajectory";
export function GPSLoggerSetup({
  credential,
  onClose,
}: {
  credential: TrajectoryCredential | null;
  onClose: () => void;
}) {
  const [notice, setNotice] = useState("");
  const c = gpsLoggerConfig(
    window.location.origin,
    credential?.token || "<创建设备后获取密钥>",
  );
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice("已复制");
    } catch {
      setNotice("复制失败，请手动选中文本复制");
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>连接 GPSLogger</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        {credential && (
          <div className="rounded-md border border-amber-400/50 bg-amber-50/50 p-3 text-foreground">
            <p className="font-medium">
              「{credential.device.name}」的上传凭证已生成
            </p>
            <p className="mt-1">
              密钥只在这里显示一次，刷新或离开页面后无法再次读取。请先保存到手机，再关闭。
            </p>
          </div>
        )}
        <p>
          在 GPSLogger 中启用「Log to custom URL」，请求方法选择{" "}
          <strong>POST</strong>。填写下面三项：
        </p>
        {[
          ["上传地址", c.url],
          ["HTTP Headers（每行一项）", c.headers],
          ["HTTP Body", c.body],
        ].map(([label, value]) => (
          <div key={label}>
            <div className="mb-1 flex items-center justify-between">
              <span className="font-medium">{label}</span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => void copy(value)}
                disabled={label.startsWith("HTTP Headers") && !credential}
              >
                <Copy className="mr-1 h-3 w-3" />
                复制
              </Button>
            </div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 text-xs">
              {value}
            </pre>
          </div>
        ))}
        {notice && <p role="status">{notice}</p>}
        {credential && (
          <Button
            variant="outline"
            onClick={() => {
              setNotice("");
              onClose();
            }}
          >
            已保存配置，隐藏密钥
          </Button>
        )}
        <ol className="list-decimal space-y-2 pl-5 text-muted-foreground">
          <li>
            正式使用必须填写你的网站 HTTPS
            地址；本地测试地址不能用于手机外网上传。
          </li>
          <li>
            开启 CSV 或 GPX 本地记录作为备份；关闭「Discard offline
            locations」，保留断网期间的数据。
          </li>
          <li>
            允许后台定位、取消电池优化，并按手机系统设置自启动。可以先尝试每 60
            秒记录一次，再根据耗电调整。
          </li>
          <li>
            停止并重新启动记录，让配置生效。首次测试后检查上方最近上传时间；网络恢复后，旧轨迹会按原定位日期补入。
          </li>
        </ol>
        <a
          className="inline-block underline underline-offset-4"
          href="https://gpslogger.app/#using-the-custom-url-feature"
          target="_blank"
          rel="noreferrer"
        >
          GPSLogger 官方配置说明
        </a>
      </CardContent>
    </Card>
  );
}
