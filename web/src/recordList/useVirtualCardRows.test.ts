import { describe, expect, it } from "vitest";

import { cardColumns } from "./useVirtualCardRows";

describe("cardColumns", () => {
  it.each([
    // 360px less the page's 12px padding on each side: two `card-record` (160px) cards.
    [336, 2],
    [171, 1],
    [172 * 3 - 12, 3],
    // Not laid out yet.
    [0, 1],
  ])("幅 %ipx には %i 列", (width, columns) => {
    expect(cardColumns(width, 160, 12)).toBe(columns);
  });
});
