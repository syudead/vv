import { describe, expect, it } from "vitest";

import { defaultAspectRatio, frameAspectRatio } from "./aspect";

describe("frameAspectRatio", () => {
  it("横長・縦長・正方形の比率をそのまま返す", () => {
    expect(frameAspectRatio(1920, 1080)).toBeCloseTo(16 / 9);
    expect(frameAspectRatio(1080, 1920)).toBeCloseTo(9 / 16);
    expect(frameAspectRatio(720, 720)).toBe(1);
  });

  it("寸法が分からなければ 16:9 を返す", () => {
    expect(frameAspectRatio(undefined, undefined)).toBe(defaultAspectRatio);
    expect(frameAspectRatio(1920, 0)).toBe(defaultAspectRatio);
    expect(frameAspectRatio(Number.NaN, 1080)).toBe(defaultAspectRatio);
  });

  it("極端に細長い比率は範囲に丸める", () => {
    expect(frameAspectRatio(100, 1000)).toBeCloseTo(9 / 21);
    expect(frameAspectRatio(1000, 100)).toBeCloseTo(21 / 9);
  });
});
