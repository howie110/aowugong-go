export const ANALYTICS_EXCLUSION_KEY = "aowugong.analytics.disabled";

// 只在应用入口启动，独立于 React 生命周期，避免 StrictMode 重复采集。
export function startAnalytics(
  win: Window & { __analyticsStarted?: boolean } = window,
) {
  if (win.__analyticsStarted) return;
  win.__analyticsStarted = true;
  let previous = "";
  let referrer = win.document.referrer;
  function track() {
    try {
      const url = new URL(win.location.href);
      const query = new URLSearchParams();
      for (const key of ["utm_source", "utm_medium", "utm_campaign"]) {
        const value = url.searchParams.get(key);
        if (value && value.length <= 100) query.set(key, value);
      }
      query.sort();
      const address =
        (url.pathname.replace(/\/+$/, "") || "/") +
        (query.size ? `?${query}` : "");
      if (address === previous) return;
      const source = referrer;
      previous = address;
      referrer = url.origin;
      try {
        if (win.localStorage.getItem(ANALYTICS_EXCLUSION_KEY) === "1") return;
      } catch {
        /* 无存储权限时仍可匿名采集。 */
      }
      let safeReferrer = "";
      try {
        const parsed = new URL(source);
        if (["http:", "https:"].includes(parsed.protocol))
          safeReferrer = parsed.origin + "/";
      } catch {
        /* 直接访问 */
      }
      const payload = {
        id: win.crypto.randomUUID(),
        url: address,
        referrer: safeReferrer,
        language: win.navigator.language || "",
      };
      void win
        .fetch("/api/v1/analytics/collect", {
          method: "POST",
          credentials: "omit",
          keepalive: true,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        })
        .catch(() => {
          /* 统计失败不阻塞业务，也不无限重试。 */
        });
    } catch {
      /* 旧浏览器或禁用 API 时降级为不采集。 */
    }
  }
  // 等同步导航守卫处理完毕后读取最终地址，被取消的后退不产生虚假浏览。
  let pending = false;
  function schedule() {
    if (pending) return;
    pending = true;
    queueMicrotask(() => {
      pending = false;
      track();
    });
  }
  for (const method of ["pushState", "replaceState"] as const) {
    const original = win.history[method];
    win.history[method] = function (...args: Parameters<History["pushState"]>) {
      const result = original.apply(this, args);
      schedule();
      return result;
    };
  }
  win.addEventListener("popstate", schedule);
  track();
}
