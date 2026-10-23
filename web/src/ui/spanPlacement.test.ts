import { describe, expect, it } from "vitest";

import { spanPlacement } from "./spanPlacement";

describe("spanPlacement", () => {
  it("60 秒の動画の 15〜30 秒は 25% の位置に幅 25% で置く", () => {
    expect(spanPlacement({ start: 15, end: 30 }, 60)).toEqual({ left: 25, width: 25 });
  });

  it("始まりより前にはみ出す区間は動画の頭に収める", () => {
    expect(spanPlacement({ start: -6, end: 6 }, 60)).toEqual({ left: 0, width: 10 });
  });

  it("終わりを越える区間は終わりまで描き、全体が越える区間は描かない", () => {
    expect(spanPlacement({ start: 45, end: 75 }, 60)).toEqual({ left: 75, width: 25 });
    expect(spanPlacement({ start: 61, end: 70 }, 60)).toBeNull();
  });
});
