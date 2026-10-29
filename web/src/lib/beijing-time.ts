const BEIJING_TIME_ZONE = "Asia/Shanghai";

/** parseBeijingTimestamp 解析后端无时区时间文本，并按北京时间解释。 */
export function parseBeijingTimestamp(value: string | Date | null | undefined): Date | null {
  if (!value) {
    return null;
  }
  if (typeof value !== "string") {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date;
  }
  const text = value.trim();
  if (!text) {
    return null;
  }
  const normalized = /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}$/.test(text)
    ? `${text.replace(" ", "T")}+08:00`
    : text;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** formatBeijingDateTime 按北京时间格式化日期时间，无法解析时返回 fallback。 */
export function formatBeijingDateTime(
  value: string | Date | null | undefined,
  options: Intl.DateTimeFormatOptions,
  fallback = "-",
  locale = "zh-CN",
): string {
  const date = parseBeijingTimestamp(value);
  if (!date) {
    return fallback;
  }
  return new Intl.DateTimeFormat(locale, { timeZone: BEIJING_TIME_ZONE, ...options }).format(date);
}

/** formatBeijingDate 按北京时间格式化为 YYYY-MM-DD。 */
export function formatBeijingDate(value: string | Date | null | undefined, fallback = "-"): string {
  return formatBeijingDateTime(
    value,
    { year: "numeric", month: "2-digit", day: "2-digit" },
    fallback,
    "sv-SE",
  );
}

/** beijingToday 返回北京时间当天 YYYY-MM-DD。 */
export function beijingToday(now = new Date()): string {
  return new Intl.DateTimeFormat("sv-SE", {
    timeZone: BEIJING_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
}

/** shiftBeijingDate 对 YYYY-MM-DD 做自然日加减。 */
export function shiftBeijingDate(value: string, days: number): string {
  const date = new Date(`${value.slice(0, 10)}T00:00:00+08:00`);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  date.setUTCDate(date.getUTCDate() + days);
  return formatBeijingDate(date, value);
}

/** formatBeijingInputDateTime 生成 datetime-local 使用的北京时间文本。 */
export function formatBeijingInputDateTime(value: Date): string {
  return formatBeijingDateTime(
    value,
    { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
    "",
    "sv-SE",
  ).replace(" ", "T");
}
