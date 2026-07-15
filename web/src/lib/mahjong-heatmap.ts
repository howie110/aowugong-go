export type MahjongHeatmapInput = {
  played_date: string;
  result_amount: string | number;
};

export type MahjongHeatmapDay = {
  date: string;
  weekday: number;
  count: number;
  resultAmount: string;
  inRange: boolean;
};

export type MahjongHeatmapWeek = {
  weekIndex: number;
  days: MahjongHeatmapDay[];
};

export type MahjongHeatmap = {
  startDate: string;
  endDate: string;
  weekdayLabels: string[];
  days: MahjongHeatmapDay[];
  weeks: MahjongHeatmapWeek[];
  totalActiveDays: number;
  totalGames: number;
  maxCount: number;
};

const weekdayLabels = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
const dayMs = 24 * 60 * 60 * 1000;

export function buildMahjongHeatmap(points: MahjongHeatmapInput[]): MahjongHeatmap {
  if (!points.length) {
    return emptyHeatmap();
  }

  const endDate = points.map((point) => point.played_date).sort().at(-1) || "";
  const startDate = formatDate(addDays(parseDate(endDate), -364));
  const dayTotals = groupPointsByDate(points, startDate, endDate);
  const days = buildRangeDays(startDate, endDate, dayTotals);
  const weeks = buildWeeks(days, startDate, endDate);
  const activeDays = days.filter((day) => day.count > 0);

  return {
    startDate,
    endDate,
    weekdayLabels,
    days,
    weeks,
    totalActiveDays: activeDays.length,
    totalGames: activeDays.reduce((sum, day) => sum + day.count, 0),
    maxCount: Math.max(...activeDays.map((day) => day.count), 0),
  };
}

export function getMahjongHeatmapCellColor(day: { count: number }) {
  return day.count > 0 ? "#18181b" : "#ffffff";
}

function emptyHeatmap(): MahjongHeatmap {
  return {
    startDate: "",
    endDate: "",
    weekdayLabels,
    days: [],
    weeks: [],
    totalActiveDays: 0,
    totalGames: 0,
    maxCount: 0,
  };
}

function groupPointsByDate(points: MahjongHeatmapInput[], startDate: string, endDate: string) {
  const totals = new Map<string, { count: number; resultAmount: number }>();
  for (const point of points) {
    if (point.played_date < startDate || point.played_date > endDate) {
      continue;
    }
    const current = totals.get(point.played_date) || { count: 0, resultAmount: 0 };
    current.count += 1;
    current.resultAmount += toNumber(point.result_amount);
    totals.set(point.played_date, current);
  }
  return totals;
}

function buildRangeDays(startDate: string, endDate: string, dayTotals: Map<string, { count: number; resultAmount: number }>) {
  const days: MahjongHeatmapDay[] = [];
  for (let date = parseDate(startDate); date <= parseDate(endDate); date = addDays(date, 1)) {
    const dateText = formatDate(date);
    const total = dayTotals.get(dateText);
    days.push({
      date: dateText,
      weekday: getMondayWeekday(date),
      count: total?.count || 0,
      resultAmount: formatMoney(total?.resultAmount || 0),
      inRange: true,
    });
  }
  return days;
}

function buildWeeks(days: MahjongHeatmapDay[], startDate: string, endDate: string) {
  const dayByDate = new Map(days.map((day) => [day.date, day]));
  const gridStart = addDays(parseDate(startDate), -getMondayWeekday(parseDate(startDate)));
  const gridEnd = addDays(parseDate(endDate), 6 - getMondayWeekday(parseDate(endDate)));
  const weeks: MahjongHeatmapWeek[] = [];
  let weekIndex = 0;

  for (let weekStart = gridStart; weekStart <= gridEnd; weekStart = addDays(weekStart, 7)) {
    const weekDays: MahjongHeatmapDay[] = [];
    for (let index = 0; index < 7; index += 1) {
      const date = addDays(weekStart, index);
      const dateText = formatDate(date);
      weekDays.push(
        dayByDate.get(dateText) || {
          date: dateText,
          weekday: getMondayWeekday(date),
          count: 0,
          resultAmount: "0.00",
          inRange: false,
        },
      );
    }
    weeks.push({ weekIndex, days: weekDays });
    weekIndex += 1;
  }

  return weeks;
}

function parseDate(value: string) {
  return new Date(`${value}T00:00:00.000Z`);
}

function addDays(value: Date, days: number) {
  return new Date(value.getTime() + days * dayMs);
}

function formatDate(value: Date) {
  return value.toISOString().slice(0, 10);
}

function getMondayWeekday(value: Date) {
  return (value.getUTCDay() + 6) % 7;
}

function toNumber(value: string | number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatMoney(value: number) {
  return value.toFixed(2);
}
