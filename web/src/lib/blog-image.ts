/** 仅转换本站图床的静态照片地址，外部图片、本地预览和可能的动图保持原样。 */
export function blogImageURL(original: string, variant: "thumb-v1" | "large-v1"): string {
  try {
    const url = new URL(original);
    if (url.origin !== "https://pic.aowugong.top" || !/^\/(pic|blog\/status)\/.+\.(jpe?g|png)$/i.test(url.pathname)) return original;
    // 不修改已有签名或其他用途的查询参数。
    if (url.search) return original;
    url.searchParams.set("variant", variant);
    return url.toString();
  } catch { return original; }
}
