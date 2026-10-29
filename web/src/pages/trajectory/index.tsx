import { formatBeijingDateTime } from "@/lib/beijing-time";
import { useEffect, useState } from "react";
import { RefreshCw, MapPin, Smartphone } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  createTrajectoryDevice,
  getTrajectory,
  getTrajectoryDevices,
  getTrajectoryProfile,
  trajectoryToday,
  type TrajectoryDevice,
  type TrajectoryFilter,
  type TrajectoryReport,
} from "@/lib/trajectory";
const time = (value: string | null) =>
  formatBeijingDateTime(value, { hour12: false }, "尚无记录");
const numeric = (value: number | null, unit = "") =>
  value === null
    ? "—"
    : `${value.toLocaleString("zh-CN", { maximumFractionDigits: 2 })}${unit}`;
export function TrajectoryPage() {
  const today = trajectoryToday();
  const [filter, setFilter] = useState<TrajectoryFilter>({
    from: today,
    through: today,
    device: "",
    page: 1,
  });
  const [from, setFrom] = useState(today);
  const [through, setThrough] = useState(today);
  const [device, setDevice] = useState("");
  const [devices, setDevices] = useState<TrajectoryDevice[]>([]);
  const [report, setReport] = useState<TrajectoryReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [pending, setPending] = useState(false);
  const [name, setName] = useState("");
  const [notice, setNotice] = useState("");
  const [copyURL, setCopyURL] = useState("");
  const [revision, setRevision] = useState(0);
  const refresh = () => {
    setFilter((f) => ({ ...f, page: 1, snapshot: undefined }));
    setRevision((n) => n + 1);
  };
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setReport(null);
    Promise.all([
      getTrajectoryDevices(controller.signal),
      getTrajectory(filter, controller.signal),
    ])
      .then(([d, r]) => {
        if (!controller.signal.aborted) {
          setDevices(d);
          setReport(r);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "读取失败");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [filter, revision]);
  async function create() {
    setPending(true);
    setActionError("");
    setNotice("");
    setCopyURL("");
    try {
      await createTrajectoryDevice(name.trim());
      setName("");
      refresh();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "创建失败");
    } finally {
      setPending(false);
    }
  }
  async function copyProfile(d: TrajectoryDevice) {
    setPending(true);
    setActionError("");
    setNotice("");
    setCopyURL("");
    try {
      // Start the clipboard operation in the click gesture, including browsers
      // that drop clipboard permission after an awaited network request.
      const link = getTrajectoryProfile(d.id).then((p) => `${window.location.origin}${p.profile_path}`);
      try {
        if (typeof ClipboardItem !== "undefined" && navigator.clipboard?.write) {
          await navigator.clipboard.write([new ClipboardItem({
            "text/plain": link.then((url) => new Blob([url], { type: "text/plain" })),
          })]);
        } else {
          await navigator.clipboard.writeText(await link);
        }
        setNotice("已复制，在 GPSLogger「从 URL」中粘贴即可。链接 15 分钟有效，原密钥继续可用。");
      } catch {
        setCopyURL(await link);
        setNotice("请复制下方配置 URL，粘贴到 GPSLogger「从 URL」。");
      }
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "生成配置失败");
    } finally {
      setPending(false);
    }
  }
  const pages = report
    ? Math.max(1, Math.ceil(report.total / report.page_size))
    : 1;
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          GPSLogger 自动记录 · 仅自己可见 · 北京时间
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={loading}
          onClick={refresh}
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          刷新
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        {[
          [
            "今日记录数",
            report ? report.today_count.toLocaleString() : "—",
            "按所选设备的定位时间统计",
          ],
          [
            "最近定位",
            report ? time(report.last_recorded_at) : "—",
            "所选设备全部历史中的最新定位",
          ],
          [
            "最近上传",
            report ? time(report.last_received_at) : "—",
            "最近收到有效上传的时间，不代表实时在线",
          ],
        ].map(([title, value, hint]) => (
          <Card key={title}>
            <CardContent className="pt-5">
              <p className="text-sm text-muted-foreground">{title}</p>
              <p className="my-3 text-xl font-semibold tabular-nums">{value}</p>
              <p className="text-xs text-muted-foreground">{hint}</p>
            </CardContent>
          </Card>
        ))}
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MapPin className="h-4 w-4" />
            轨迹记录
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (!from || !through || through < from) {
                setError("请选择有效日期范围");
                return;
              }
              setFilter({ from, through, device, page: 1 });
            }}
          >
            <label className="space-y-1 text-sm">
              设备
              <select
                aria-label="轨迹设备"
                value={device}
                onChange={(e) => setDevice(e.target.value)}
                className="block h-9 rounded-md border bg-background px-3"
              >
                <option value="">全部设备</option>
                {devices.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                    {d.revoked_at ? "（已撤销）" : ""}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-1 text-sm">
              开始日期
              <Input
                type="date"
                aria-label="轨迹开始日期"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
                required
              />
            </label>
            <label className="space-y-1 text-sm">
              结束日期（含）
              <Input
                type="date"
                aria-label="轨迹结束日期"
                value={through}
                min={from}
                onChange={(e) => setThrough(e.target.value)}
                required
              />
            </label>
            <Button type="submit" disabled={loading}>
              查询
            </Button>
          </form>
          <p className="text-xs text-muted-foreground">
            {filter.from} 至 {filter.through} · 最多查询 366 天 · 坐标为
            WGS84；低精度点也保留，不自动修正漂移。
          </p>
          {error ? (
            <div
              role="alert"
              className="rounded-md border p-4 text-sm text-destructive"
            >
              {error}
              <Button variant="outline" className="ml-3" onClick={refresh}>
                重试
              </Button>
            </div>
          ) : loading ? (
            <p
              role="status"
              className="py-10 text-center text-muted-foreground"
            >
              正在读取轨迹…
            </p>
          ) : report?.points.length ? (
            <>
              <div className="overflow-x-auto">
                <table className="w-full whitespace-nowrap text-left text-sm">
                  <thead>
                    <tr className="border-b text-muted-foreground">
                      {[
                        "定位时间",
                        "设备",
                        "纬度 / 经度",
                        "精度",
                        "速度",
                        "海拔",
                        "方向",
                        "接收时间",
                      ].map((h) => (
                        <th key={h} className="px-3 py-3 font-medium">
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {report.points.map((p) => (
                      <tr key={p.id} className="border-b">
                        <td className="px-3 py-3 tabular-nums">
                          {time(p.recorded_at)}
                        </td>
                        <td className="px-3 py-3">{p.device_name}</td>
                        <td className="px-3 py-3 font-mono text-xs">
                          {p.lat.toFixed(6)} / {p.lon.toFixed(6)}
                        </td>
                        <td className="px-3 py-3">
                          {numeric(p.accuracy, " m")}
                        </td>
                        <td className="px-3 py-3">
                          {numeric(
                            p.speed === null ? null : p.speed * 3.6,
                            " km/h",
                          )}
                        </td>
                        <td className="px-3 py-3">
                          {numeric(p.altitude, " m")}
                        </td>
                        <td className="px-3 py-3">{numeric(p.bearing, "°")}</td>
                        <td className="px-3 py-3 tabular-nums">
                          {time(p.received_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
                <span>
                  共 {report.total.toLocaleString()} 条 · 第 {filter.page} /{" "}
                  {pages} 页
                </span>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    disabled={filter.page <= 1}
                    onClick={() =>
                      setFilter((f) => ({
                        ...f,
                        page: f.page - 1,
                        snapshot: report.snapshot,
                      }))
                    }
                  >
                    上一页
                  </Button>
                  <Button
                    variant="outline"
                    disabled={filter.page >= pages}
                    onClick={() =>
                      setFilter((f) => ({
                        ...f,
                        page: f.page + 1,
                        snapshot: report.snapshot,
                      }))
                    }
                  >
                    下一页
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <p className="py-10 text-center text-muted-foreground">
              这段时间没有轨迹记录。创建一个设备并配置 GPSLogger
              后，记录会出现在这里。
            </p>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Smartphone className="h-4 w-4" />
            记录设备
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void create();
            }}
          >
            <Input
              aria-label="新设备名称"
              placeholder="例如：我的安卓手机"
              value={name}
              maxLength={60}
              onChange={(e) => setName(e.target.value)}
              className="max-w-sm"
            />
            <Button
              disabled={pending || !name.trim()}
              type="submit"
            >
              添加设备
            </Button>
          </form>
          {actionError && (
            <p role="alert" className="text-sm text-destructive">
              {actionError}
            </p>
          )}
          {devices.map((d) => (
            <div
              key={d.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3"
            >
              <div>
                <p className="font-medium">
                  {d.name}{" "}
                  <span className="text-xs text-muted-foreground">
                    {d.revoked_at ? "已撤销" : "允许上传"}
                  </span>
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  最近上传：{time(d.last_received_at)}
                </p>
              </div>
              {!d.revoked_at && (
                <Button size="sm" variant="outline" disabled={pending} onClick={() => void copyProfile(d)}>
                  复制配置 URL
                </Button>
              )}
            </div>
          ))}
          {notice && <p role="status" className="text-sm text-muted-foreground">{notice}</p>}
          {copyURL && <Input aria-label="配置 URL" readOnly value={copyURL} onFocus={(e) => e.target.select()} />}
        </CardContent>
      </Card>
    </div>
  );
}
