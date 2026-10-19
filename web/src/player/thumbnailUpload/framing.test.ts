import { describe, expect, it } from "vitest";

import {
  compositionSize,
  coversFrame,
  fillFrame,
  frameAspect,
  frameSize,
  panBy,
  placeImage,
  rotateBy,
  snapToFrame,
  WHOLE_IMAGE,
  wholeImage,
  zoomTo,
} from "./framing";

const wide = { width: 2048, height: 1152 };
const portrait = { width: 1000, height: 2000 };

describe("frameAspect", () => {
  it("表示上の比率、なければ幅と高さ、どちらも無ければ null を返す", () => {
    expect(frameAspect({ displayAspectRatio: 2, width: 100, height: 100 })).toBe(2);
    expect(frameAspect({ width: 1080, height: 1920 })).toBeCloseTo(0.5625);
    expect(frameAspect({ displayAspectRatio: 0, width: 400, height: 200 })).toBe(2);
    expect(frameAspect({ width: 0, height: 100 })).toBeNull();
    expect(frameAspect({})).toBeNull();
  });
});

describe("frameSize", () => {
  it("動画の比率で長辺 2048px、比率が分からなければ回した後の画像の比率にする", () => {
    expect(compositionSize({ width: 10, height: 10 }, 16 / 9)).toEqual(wide);
    expect(frameSize(portrait, 9 / 16, 90)).toEqual({ width: 1152, height: 2048 });
    expect(frameSize({ width: 600, height: 300 }, null, 0)).toEqual({
      width: 2048,
      height: 1024,
    });
    expect(frameSize({ width: 600, height: 300 }, null, 270)).toEqual({
      width: 1024,
      height: 2048,
    });
  });
});

describe("placeImage", () => {
  it("等倍は全体が見える大きさで中央に置き、比率が 1px 以内で同じなら枠いっぱいにする", () => {
    expect(placeImage(portrait, wide, WHOLE_IMAGE)).toEqual({
      x: 736,
      y: 0,
      width: 576,
      height: 1152,
    });
    expect(placeImage({ width: 1921, height: 1080 }, wide, WHOLE_IMAGE)).toEqual({
      x: 0,
      y: 0,
      ...wide,
    });
  });

  it("位置は枠の幅と高さに対する割合で、拡大は全体が見える大きさの倍率で決まる", () => {
    expect(
      placeImage(portrait, wide, { ...WHOLE_IMAGE, x: 0.25, y: -0.5, zoom: 2 }),
    ).toEqual({ x: 448 + 512, y: -576 - 576, width: 1152, height: 2304 });
  });

  it("4 分の 1 回すと縦横を入れ替えた大きさで置く", () => {
    expect(placeImage(portrait, wide, { ...WHOLE_IMAGE, rotation: 90 })).toEqual({
      x: 0,
      y: (1152 - 1024) / 2,
      width: 2048,
      height: 1024,
    });
  });
});

describe("fillFrame と coversFrame", () => {
  it("枠を埋めると中央で枠をちょうど覆い、回した後の向きで倍率を決める", () => {
    const filled = fillFrame(portrait, wide, { ...WHOLE_IMAGE, x: 0.3 });
    expect(filled.x).toBe(0);
    expect(placeImage(portrait, wide, filled).width).toBeCloseTo(2048);
    expect(coversFrame(portrait, wide, filled)).toBe(true);
    const turnedFill = fillFrame(portrait, wide, { ...WHOLE_IMAGE, rotation: 270 });
    expect(turnedFill.zoom).toBeCloseTo(1.125);
    expect(coversFrame(portrait, wide, turnedFill)).toBe(true);
  });

  it("全体表示に戻すと位置と倍率だけが初めに戻り、余白が出る", () => {
    const moved = { ...WHOLE_IMAGE, x: 0.2, zoom: 3, rotation: 180 as const };
    expect(wholeImage(moved)).toEqual({ ...moved, x: 0, zoom: 1 });
    expect(coversFrame(portrait, wide, WHOLE_IMAGE)).toBe(false);
  });
});

describe("panBy", () => {
  it("どの向きにも動かせ、画像の一部が枠に残るところで止まる", () => {
    const moved = panBy(portrait, wide, WHOLE_IMAGE, 204.8, -115.2);
    expect(moved.x).toBeCloseTo(0.1);
    expect(moved.y).toBeCloseTo(-0.1);
    const far = panBy(portrait, wide, WHOLE_IMAGE, 1e6, 1e6);
    expect(far.x).toBeCloseTo(0.5 + 576 / 2048 / 2);
    expect(far.y).toBeCloseTo(1);
  });
});

describe("snapToFrame", () => {
  it("中央や枠の端に近ければ合わせ、遠ければそのままにする", () => {
    const near = snapToFrame(portrait, wide, { ...WHOLE_IMAGE, x: 10 / 2048 }, 30);
    expect(near.x).toBe(0);
    // A 576px-wide image's right edge meets the frame's at x = (2048 - 576) / 2.
    const right = snapToFrame(portrait, wide, { ...WHOLE_IMAGE, x: 720 / 2048 }, 30);
    expect(right.x).toBeCloseTo(736 / 2048);
    const free = snapToFrame(portrait, wide, { ...WHOLE_IMAGE, x: 400 / 2048 }, 30);
    expect(free.x).toBeCloseTo(400 / 2048);
  });
});

describe("zoomTo と rotateBy", () => {
  it("拡大は枠の中央を中心にし、倍率を 0.25〜16 倍に収める", () => {
    expect(zoomTo({ ...WHOLE_IMAGE, x: 0.1, y: -0.2 }, 2)).toEqual({
      ...WHOLE_IMAGE,
      x: 0.2,
      y: -0.4,
      zoom: 2,
    });
    expect(zoomTo(WHOLE_IMAGE, 100).zoom).toBe(16);
    expect(zoomTo(WHOLE_IMAGE, 0.01).zoom).toBe(0.25);
  });

  it("回転は 4 分の 1 ずつで一周する", () => {
    expect(rotateBy(WHOLE_IMAGE, -90).rotation).toBe(270);
    expect(rotateBy({ ...WHOLE_IMAGE, rotation: 270 }, 90).rotation).toBe(0);
  });
});
