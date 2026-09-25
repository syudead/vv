import { describe, expect, it } from "vitest";

import { defaultAspectRatio, frameAspectRatio, playerAspectRatio } from "./aspect";

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
    expect(frameAspectRatio(100, 1000)).toBeCloseTo(9 / 16);
    expect(frameAspectRatio(1080, 2340)).toBeCloseTo(9 / 16);
    expect(frameAspectRatio(1000, 100)).toBeCloseTo(21 / 9);
  });
});

describe("playerAspectRatio", () => {
  const anamorphic = { width: 720, height: 576, displayAspectRatio: 4 / 3 };

  it("再生で分かった比率を最優先する", () => {
    expect(playerAspectRatio(anamorphic, 9 / 16)).toBeCloseTo(9 / 16);
  });

  it("次に解析の表示比率を使い、画素の縦横比を反映する", () => {
    expect(playerAspectRatio(anamorphic, null)).toBeCloseTo(4 / 3);
  });

  it("表示比率が無い既存の動画は解像度の比を使う", () => {
    expect(playerAspectRatio({ width: 1080, height: 1920 }, null)).toBeCloseTo(9 / 16);
    expect(playerAspectRatio(undefined, null)).toBe(defaultAspectRatio);
  });
});
