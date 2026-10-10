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

  it("1 行目は今日・昨日・それより前の曜日、2 行目は月と日", () => {
    expect(dayLabel(new Date(2026, 8, 27), now)).toEqual({
      name: "Today",
      date: "Sep 27",
    });
    expect(dayLabel(new Date(2026, 8, 26), now)).toEqual({
      name: "Yesterday",
      date: "Sep 26",
    });
    expect(dayLabel(new Date(2026, 8, 25), now)).toEqual({
      name: "Friday",
      date: "Sep 25",
    });
  });

  it("今年でない日は 2 行目に年も書く", () => {
    expect(dayLabel(new Date(2025, 9, 7), now)).toEqual({
      name: "Tuesday",
      date: "Oct 7, 2025",
    });
  });

  it("月をまたいでも前の日を昨日にする", () => {
    expect(dayLabel(new Date(2026, 8, 30), new Date(2026, 9, 1, 8, 0)).name).toBe(
      "Yesterday",
    );
  });
});

describe("msUntilNextMidnight", () => {
  it("次のローカルの 0 時までの時間を返す", () => {
    expect(msUntilNextMidnight(new Date(2026, 8, 27, 23, 59, 0))).toBe(60_000);
  });
});
