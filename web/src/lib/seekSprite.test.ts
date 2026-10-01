import { describe, expect, it } from "vitest";

import type { SeekThumbnailSprite } from "../api/client";
import { seekPosition, seekSpriteCell, spriteCellBackground } from "./seekSprite";

const rect = { left: 100, width: 400 };

// 2 時間の動画の配置情報（12 秒間隔・600 コマ・10×10 の 6 シート）。
function twoHourSprite(): SeekThumbnailSprite {
  return {
    intervalMs: 12_000,
    frameCount: 600,
    columns: 10,
    rows: 10,
    frameWidth: 320,
    frameHeight: 180,
    sheets: [0, 1, 2, 3, 4, 5].map(
      (n) => `/api/videos/1/seek-thumbnail/${String(n)}?v=c`,
    ),
  };
}

describe("seekPosition", () => {
  it("左端を0ms、右端をceil(durationMs) - 1にし、矩形の外は端に丸める", () => {
    expect(seekPosition(100, rect, 120_000)).toEqual({
      localX: 0,
      ratio: 0,
      positionMs: 0,
    });
    expect(seekPosition(300, rect, 120_000)).toEqual({
      localX: 200,
      ratio: 0.5,
      positionMs: 60_000,
    });
    expect(seekPosition(500, rect, 120_000)).toEqual({
      localX: 400,
      ratio: 1,
      positionMs: 119_999,
    });
    expect(seekPosition(40, rect, 120_000).positionMs).toBe(0);
    expect(seekPosition(900, rect, 120_000)).toEqual({
      localX: 400,
      ratio: 1,
      positionMs: 119_999,
    });
    // 端数のある長さでも、右端は ceil で丸めた末尾の 1ms 手前になる。
    expect(seekPosition(500, rect, 10_000.4).positionMs).toBe(10_000);
  });

  it("幅の無い矩形では先頭を指す", () => {
    expect(seekPosition(100, { left: 100, width: 0 }, 120_000).positionMs).toBe(0);
  });
});

describe("seekSpriteCell", () => {
  it("帯の右端の位置で末尾のコマを指し、frameCount - 1を超えない", () => {
    // 9 コマ（3×3 の 1 シート）で、最後のコマの区間がちょうど末尾で終わる長さ。
    const sprite = {
      intervalMs: 5_000,
      frameCount: 9,
      columns: 3,
      rows: 3,
    };
    const durationMs = 45_000;
    const { positionMs } = seekPosition(500, rect, durationMs);
    expect(positionMs).toBe(44_999);
    expect(seekSpriteCell(sprite, positionMs)).toEqual({
      frame: 8,
      sheet: 0,
      column: 2,
      row: 2,
    });
    // シートに空き升目が残る（frameCount < columns × rows）ときも空き升目を指さない。
    const partial = { ...sprite, frameCount: 7 };
    expect(seekSpriteCell(partial, 44_999)).toEqual({
      frame: 6,
      sheet: 0,
      column: 0,
      row: 2,
    });
    expect(seekSpriteCell(partial, Number.MAX_SAFE_INTEGER).frame).toBe(6);
  });

  it("複数シートでは、指した位置のコマが載るシートの番号になる", () => {
    const sprite = twoHourSprite();
    const durationMs = 7_200_000;
    expect(
      seekSpriteCell(sprite, seekPosition(100, rect, durationMs).positionMs).sheet,
    ).toBe(0);
    // 帯の真ん中は 3_600_000ms = 300 コマ目で、4 枚目（番号 3）の先頭。
    expect(
      seekSpriteCell(sprite, seekPosition(300, rect, durationMs).positionMs),
    ).toEqual({
      frame: 300,
      sheet: 3,
      column: 0,
      row: 0,
    });
    expect(seekSpriteCell(sprite, 1_199_999)).toEqual({
      frame: 99,
      sheet: 0,
      column: 9,
      row: 9,
    });
    expect(seekSpriteCell(sprite, 1_200_000).sheet).toBe(1);
    expect(
      seekSpriteCell(sprite, seekPosition(500, rect, durationMs).positionMs),
    ).toEqual({
      frame: 599,
      sheet: 5,
      column: 9,
      row: 9,
    });
  });
});

describe("spriteCellBackground", () => {
  it("シートを列・行の数だけ広げて敷き、升目の分だけずらす", () => {
    const layout = { columns: 10, rows: 10 };
    expect(spriteCellBackground({ column: 0, row: 0 }, layout)).toEqual({
      backgroundSize: "1000% 1000%",
      backgroundPosition: "0% 0%",
    });
    expect(spriteCellBackground({ column: 9, row: 9 }, layout)).toEqual({
      backgroundSize: "1000% 1000%",
      backgroundPosition: "100% 100%",
    });
    expect(spriteCellBackground({ column: 0, row: 0 }, { columns: 1, rows: 1 })).toEqual({
      backgroundSize: "100% 100%",
      backgroundPosition: "0% 0%",
    });
  });
});
