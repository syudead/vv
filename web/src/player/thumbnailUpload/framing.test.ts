import { describe, expect, it } from "vitest";

import {
  CENTERED_COVER,
  compositionSize,
  frameAspect,
  panBy,
  placeImage,
  zoomTo,
} from "./framing";

const wide = { width: 2048, height: 1152 };

describe("frameAspect", () => {
  it("表示上の比率、なければ幅と高さ、どちらも無ければ null を返す", () => {
    expect(frameAspect({ displayAspectRatio: 2, width: 100, height: 100 })).toBe(2);
    expect(frameAspect({ width: 1080, height: 1920 })).toBeCloseTo(0.5625);
    expect(frameAspect({})).toBeNull();
  });
});

describe("compositionSize", () => {
  it("動画の比率で長辺 2048px、比率が分からなければ画像自身の比率にする", () => {
    expect(compositionSize({ width: 10, height: 10 }, 16 / 9)).toEqual(wide);
    expect(compositionSize({ width: 10, height: 10 }, 9 / 16)).toEqual({
      width: 1152,
      height: 2048,
    });
    expect(compositionSize({ width: 300, height: 600 }, null)).toEqual({
      width: 1024,
      height: 2048,
    });
  });
});

describe("placeImage", () => {
  it("contain は縦長の画像を全体が見える大きさで中央に置く", () => {
    expect(placeImage({ width: 1000, height: 2000 }, wide, { mode: "contain" })).toEqual({
      x: 736,
      y: 0,
      width: 576,
      height: 1152,
    });
  });

  it("contain は比率が 1px 以内で同じ画像を枠いっぱいに描き、余白の線を出さない", () => {
    for (const image of [
      { width: 1920, height: 1080 },
      { width: 1921, height: 1080 },
    ])
      expect(placeImage(image, wide, { mode: "contain" })).toEqual({
        x: 0,
        y: 0,
        ...wide,
      });
  });

  it("cover は等倍で中央の帯、4 倍でその 4 分の 1 を見せる", () => {
    const image = { width: 1000, height: 2000 };
    const centered = placeImage(image, wide, CENTERED_COVER);
    expect(centered.width).toBeCloseTo(2048);
    expect(centered.height).toBeCloseTo(4096);
    expect(centered.y).toBeCloseTo((1152 - 4096) / 2);
    const zoomed = placeImage(image, wide, zoomTo(CENTERED_COVER, 4));
    expect(zoomed.width).toBeCloseTo(8192);
    expect(zoomed.x).toBeCloseTo((2048 - 8192) / 2);
  });

  it("cover の拡大率は 1〜4 倍に収める", () => {
    expect(zoomTo(CENTERED_COVER, 9)).toMatchObject({ zoom: 4 });
    expect(zoomTo(CENTERED_COVER, 0.2)).toMatchObject({ zoom: 1 });
  });
});

describe("panBy", () => {
  const image = { width: 1000, height: 2000 };

  it("動かした量だけ画像が動き、枠を覆ったまま端で止まる", () => {
    const moved = panBy(image, wide, CENTERED_COVER, 0, 100);
    expect(placeImage(image, wide, moved).y).toBeCloseTo((1152 - 4096) / 2 + 100);
    const top = panBy(image, wide, CENTERED_COVER, 0, 1e6);
    expect(placeImage(image, wide, top).y).toBeCloseTo(0);
    const bottom = panBy(image, wide, CENTERED_COVER, 0, -1e6);
    expect(placeImage(image, wide, bottom).y).toBeCloseTo(1152 - 4096);
  });

  it("動かせる幅の無い向きと contain は動かない", () => {
    expect(panBy(image, wide, CENTERED_COVER, 500, 0)).toMatchObject({ panX: 0 });
    expect(panBy(image, wide, { mode: "contain" }, 10, 10)).toEqual({ mode: "contain" });
  });
});
