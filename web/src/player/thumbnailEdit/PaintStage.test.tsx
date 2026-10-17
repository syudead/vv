import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Stroke } from "./mask";
import PaintStage from "./PaintStage";

// 塗りの層で引いた筆跡を、画面の座標からコマの画素へ移して渡すこと
// （specs/045-comfyui-thumbnails/ui-design.md「Compose」）。マスクはこの筆跡から作るので、
// 移し方が狂うと塗った場所と違うところが変わる。jsdom には canvas の描画が無いので、
// 描画は無いものとし、渡された筆跡を見る。

function renderStage(props: Partial<Parameters<typeof PaintStage>[0]> = {}) {
  const onStroke = vi.fn<(stroke: Stroke) => void>();
  render(
    <PaintStage
      frameUrl="/api/thumbnail-edits/e1/frame"
      frameWidth={1920}
      frameHeight={1080}
      strokes={[]}
      mode="paint"
      brushSize={54}
      locked={false}
      onStroke={onStroke}
      onUndo={vi.fn()}
      {...props}
    />,
  );
  return { onStroke, layer: screen.getByRole("img", { name: "Painted area" }) };
}

describe("PaintStage", () => {
  beforeEach(() => {
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    // 層は画面の (100, 50) から 480×270 に見え、コマ（1920×1080）の 4 分の 1 の大きさである。
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 50,
      left: 100,
      top: 50,
      right: 580,
      bottom: 320,
      width: 480,
      height: 270,
      toJSON: () => ({}),
    });
    HTMLElement.prototype.setPointerCapture = vi.fn();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("引いた筆跡を、層の中の割合のままコマの画素の点にして、離したときに 1 回渡す", () => {
    const { onStroke, layer } = renderStage();
    // 層の (0.3, 0.3) から (0.6, 0.6) まで（割合）を一筆で引く。
    const at = (x: number, y: number) => ({
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      clientX: 100 + 480 * x,
      clientY: 50 + 270 * y,
    });
    fireEvent.pointerDown(layer, at(0.3, 0.3));
    fireEvent.pointerMove(layer, at(0.45, 0.45));
    // 別の指の動きは筆跡に入れない。
    fireEvent.pointerMove(layer, { ...at(0.9, 0.1), pointerId: 2 });
    fireEvent.pointerMove(layer, at(0.6, 0.6));
    expect(onStroke).not.toHaveBeenCalled();
    fireEvent.pointerUp(layer, at(0.6, 0.6));

    expect(onStroke).toHaveBeenCalledTimes(1);
    const stroke = onStroke.mock.calls[0]![0];
    expect(stroke.mode).toBe("paint");
    expect(stroke.size).toBe(54);
    const expected = [
      { x: 576, y: 324 },
      { x: 864, y: 486 },
      { x: 1152, y: 648 },
    ];
    expect(stroke.points).toHaveLength(expected.length);
    stroke.points.forEach((point, index) => {
      expect(point.x).toBeCloseTo(expected[index]!.x);
      expect(point.y).toBeCloseTo(expected[index]!.y);
    });
  });

  it("固まっている間は塗らない", () => {
    const { onStroke, layer } = renderStage({ locked: true });
    fireEvent.pointerDown(layer, { pointerId: 1, pointerType: "mouse", button: 0 });
    fireEvent.pointerUp(layer, { pointerId: 1 });
    expect(onStroke).not.toHaveBeenCalled();
  });
});
