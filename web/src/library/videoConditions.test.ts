import { describe, expect, it } from "vitest";

import {
  durationText,
  parseVideoConditions,
  videoConditionChips,
} from "./videoConditions";

describe("parseVideoConditions", () => {
  it("サーバーが拒む値と、順序が逆の組は読まない", () => {
    expect(
      parseVideoConditions(
        new URLSearchParams(
          "minDurationMs=-1&maxDurationMs=60000&addedFrom=2025-02-30&addedTo=2025-03-01&watchState=seen",
        ),
      ),
    ).toEqual({ maxDurationMs: 60_000, addedTo: "2025-03-01" });
    expect(
      parseVideoConditions(
        new URLSearchParams(
          "minDurationMs=2000&maxDurationMs=1000&addedFrom=2025-03-02&addedTo=2025-03-01",
        ),
      ),
    ).toEqual({});
  });
});

describe("chip texts", () => {
  it("長さを分に丸めて時間と分で表す", () => {
    expect([600_000, 3_600_000, 9_000_000, 3_659_000].map(durationText)).toEqual([
      "10 min",
      "1 h",
      "2 h 30 min",
      "1 h 1 min",
    ]);
  });

  it("片側だけの範囲はその向きの文で表す", () => {
    expect(
      videoConditionChips({ maxDurationMs: 600_000, addedTo: "2025-03-01" }).map(
        (chip) => chip.text,
      ),
    ).toEqual(["Up to 10 min", "Added until Mar 1, 2025"]);
  });
});
