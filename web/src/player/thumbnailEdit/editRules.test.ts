import { describe, expect, it } from "vitest";

import {
  brushDiameter,
  elapsedNow,
  fitBox,
  isRunning,
  referenceAccepted,
  runStateText,
  shapeDiffers,
  showsLongWait,
  sourceWords,
} from "./editRules";

describe("editRules", () => {
  it("待ち行列・準備中・生成中だけを実行中とし、状態の文言を返す", () => {
    expect(["queued", "preparing", "running"].every((s) => isRunning(s as never))).toBe(
      true,
    );
    for (const state of ["ready", "succeeded", "failed", "cancelled"] as const)
      expect(isRunning(state)).toBe(false);
    expect(runStateText("queued")).toBe("Waiting in the queue");
    expect(runStateText("preparing")).toBe("Preparing");
    expect(runStateText("running")).toBe("Generating");
  });

  it("長い待ちの説明は、待ち行列か準備中で 20 秒を過ぎてから出す", () => {
    expect(showsLongWait("queued", 19_999)).toBe(false);
    expect(showsLongWait("queued", 20_000)).toBe(true);
    expect(showsLongWait("preparing", 25_000)).toBe(true);
    expect(showsLongWait("running", 60_000)).toBe(false);
  });

  it("経過時間はサーバーの値に、受け取ってからの時間を足す", () => {
    expect(elapsedNow(4_000, 10_000, 12_500)).toBe(6_500);
    // 時計が戻っても、サーバーの値より小さくしない。
    expect(elapsedNow(4_000, 10_000, 9_000)).toBe(4_000);
  });

  it("ブラシの直径はコマの短い辺の 2%・5%・10% で、段の外は端に寄せる", () => {
    expect(brushDiameter(0, 1920, 1080)).toBeCloseTo(21.6);
    expect(brushDiameter(1, 1920, 1080)).toBeCloseTo(54);
    expect(brushDiameter(2, 1080, 1920)).toBeCloseTo(108);
    expect(brushDiameter(5, 1920, 1080)).toBeCloseTo(108);
    expect(brushDiameter(-1, 1920, 1080)).toBeCloseTo(21.6);
  });

  it("参考画像は PNG・JPEG・WebP で 20 MB までを受け付ける", () => {
    const mb = 1024 * 1024;
    expect(referenceAccepted({ type: "image/png", size: 20 * mb })).toBe(true);
    expect(referenceAccepted({ type: "image/jpeg", size: 1 })).toBe(true);
    expect(referenceAccepted({ type: "image/webp", size: 1 })).toBe(true);
    expect(referenceAccepted({ type: "image/png", size: 20 * mb + 1 })).toBe(false);
    expect(referenceAccepted({ type: "image/gif", size: 1 })).toBe(false);
    expect(referenceAccepted({ type: "", size: 1 })).toBe(false);
  });

  it("生成した画像の形がコマと 1% より違うときだけ、切り抜いたと書き添える", () => {
    expect(shapeDiffers(1920, 1080, 1024, 576)).toBe(false);
    expect(shapeDiffers(1920, 1080, 1024, 580)).toBe(false);
    expect(shapeDiffers(1920, 1080, 1024, 1024)).toBe(true);
    expect(shapeDiffers(1920, 1080, 0, 0)).toBe(false);
  });

  it("箱は比率を保って、全体が見えるいちばん大きな大きさに収める", () => {
    expect(fitBox(1600, 600, 2)).toEqual({ width: 1200, height: 600 });
    expect(fitBox(320, 600, 16 / 9)).toEqual({ width: 320, height: 180 });
    expect(fitBox(0, 600, 16 / 9)).toEqual({ width: 0, height: 0 });
  });

  it("編集画面の文言を応答の source と positionMs だけから決める", () => {
    expect(sourceWords({ source: "frame", positionMs: 83_456 })).toEqual({
      detail: "Frame at 1:23",
      paintingHint:
        "Paint over what to change. Leave it unpainted to change the whole frame.",
      sourceLabel: "Frame",
    });
    expect(sourceWords({ source: "thumbnail", positionMs: 83_456 })).toEqual({
      detail: "Current thumbnail, frame at 1:23",
      paintingHint:
        "Paint over what to change. Leave it unpainted to change the whole image.",
      sourceLabel: "Thumbnail",
    });
    expect(sourceWords({ source: "thumbnail" }).detail).toBe(
      "Current thumbnail, saved image",
    );
  });
});
