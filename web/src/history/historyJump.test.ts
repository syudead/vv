import { describe, expect, it } from "vitest";

import { firstTarget, jumpTargets } from "./historyJump";

// 日付へ移る一覧の項目（specs/043-watch-history/ui-design.md「Jump to date」）。

const now = new Date(2026, 9, 10, 12, 0);

describe("jumpTargets", () => {
  it("直近 14 日は日、それより前は月にまとめ、どちらも新しい順に並べる", () => {
    const targets = jumpTargets(
      [
        "2026-10-10",
        "2026-10-09",
        "2026-10-07",
        "2026-09-27",
        "2026-09-26",
        "2026-09-03",
        "2026-08-20",
        "2026-08-03",
        "2025-12-31",
      ],
      now,
    );
    expect(targets.days.map(({ value, label }) => [value, label])).toEqual([
      ["2026-10-10", "Today"],
      ["2026-10-09", "Yesterday"],
      ["2026-10-07", "Wed, Oct 7"],
      ["2026-09-27", "Sun, Sep 27"],
    ]);
    // 9 月 26 日は 14 日より前なので、9 月の月の項目になる。
    expect(targets.months.map(({ value, label }) => [value, label])).toEqual([
      ["2026-09", "September"],
      ["2026-08", "August"],
      ["2025-12", "December 2025"],
    ]);
    expect(firstTarget(targets)).toBe("2026-10-10");
  });

  it("日が 14 日の内にしか無ければ月を並べず、14 日より前だけなら最初の項目は月", () => {
    expect(jumpTargets(["2026-10-01"], now).months).toEqual([]);
    const older = jumpTargets(["2026-07-01", "bad"], now);
    expect(older.days).toEqual([]);
    expect(firstTarget(older)).toBe("2026-07");
    expect(firstTarget(jumpTargets([], now))).toBeUndefined();
  });
});
