import assert from "node:assert/strict";

import { buildMahjongHeatmap, getMahjongHeatmapCellColor } from "./mahjong-heatmap";

function point(sequence: number, playedDate: string, resultAmount: string) {
  return {
    sequence,
    played_date: playedDate,
    result_amount: resultAmount,
    cumulative_result: resultAmount,
    running_average: resultAmount,
  };
}

const heatmap = buildMahjongHeatmap([
  point(1, "2025-01-01", "10.00"),
  point(2, "2025-03-03", "-12.00"),
  point(3, "2026-01-01", "20.00"),
  point(4, "2026-01-01", "-5.00"),
]);

assert.equal(heatmap.weekdayLabels.join(","), "周一,周二,周三,周四,周五,周六,周日");
assert.equal(heatmap.startDate, "2025-01-02");
assert.equal(heatmap.endDate, "2026-01-01");
assert.equal(heatmap.weeks.length, 53);

const oldDay = heatmap.days.find((day) => day.date === "2025-01-01");
assert.equal(oldDay, undefined);

const monday = heatmap.days.find((day) => day.date === "2025-03-03");
assert.equal(monday?.weekday, 0);
assert.equal(monday?.count, 1);
assert.equal(monday?.resultAmount, "-12.00");

const latest = heatmap.days.find((day) => day.date === "2026-01-01");
assert.equal(latest?.count, 2);
assert.equal(latest?.resultAmount, "15.00");
assert.equal(heatmap.totalActiveDays, 2);
assert.equal(heatmap.totalGames, 3);
assert.equal(getMahjongHeatmapCellColor({ count: 0 }), "#ffffff");
assert.equal(getMahjongHeatmapCellColor({ count: 1 }), "#18181b");
assert.equal(getMahjongHeatmapCellColor({ count: 3 }), "#18181b");
