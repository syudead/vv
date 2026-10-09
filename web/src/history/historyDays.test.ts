import { describe, expect, it } from "vitest";

import type { WatchHistoryEntry } from "../api/history";
import { dayLabel, groupByDay, msUntilNextMidnight } from "./historyDays";

function entry(id: number, playedAt: Date): WatchHistoryEntry {
  return { id, playedAt: playedAt.toISOString(), title: `Entry ${String(id)}` };
}

describe("groupByDay", () => {
  it("ローカルの 0 時で日を分け、API の順を保つ", () => {
    const entries = [
      entry(4, new Date(2026, 8, 28, 0, 10)),
      entry(3, new Date(2026, 8, 27, 23, 50)),
      entry(2, new Date(2026, 8, 27, 9, 0)),
      entry(1, new Date(2026, 8, 25, 12, 0)),
    ];

    const days = groupByDay(entries);

    expect(days.map((day) => day.entries.map((item) => item.id))).toEqual([
      [4],
      [3, 2],
      [1],
    ]);
    expect(days[1]?.day).toEqual(new Date(2026, 8, 27));
  });

  it("続きのページの同じ日の件は開いているまとまりに加わる", () => {
    const first = [entry(3, new Date(2026, 8, 27, 15, 0))];
    const next = [entry(2, new Date(2026, 8, 27, 9, 0)), entry(1, new Date(2026, 8, 26))];

    const days = groupByDay([...first, ...next]);

    expect(days.map((day) => day.entries.length)).toEqual([2, 1]);
  });

  it("読めない時刻の件はまとめない", () => {
    expect(groupByDay([{ id: 1, playedAt: "nope", title: "" }])).toEqual([]);
  });
});

describe("dayLabel", () => {
  const now = new Date(2026, 8, 27, 0, 5);

  it("今日・昨日・それより前の日付を返す", () => {
    expect(dayLabel(new Date(2026, 8, 27), now)).toBe("Today");
    expect(dayLabel(new Date(2026, 8, 26), now)).toBe("Yesterday");
    expect(dayLabel(new Date(2026, 8, 25), now)).toBe("Sep 25, 2026");
  });

  it("月をまたいでも前の日を昨日にする", () => {
    expect(dayLabel(new Date(2026, 8, 30), new Date(2026, 9, 1, 8, 0))).toBe("Yesterday");
  });
});

describe("msUntilNextMidnight", () => {
  it("次のローカルの 0 時までの時間を返す", () => {
    expect(msUntilNextMidnight(new Date(2026, 8, 27, 23, 59, 0))).toBe(60_000);
  });
});
