import { afterEach, describe, expect, it, vi } from "vitest";

import { buildMask, drawStroke, type Stroke } from "./mask";

// 送るマスクの組み立て（specs/045-comfyui-thumbnails/contracts/workflow.md「Workflows」）。
// マスクはコマの大きさで、塗った筆跡を白、残りを黒で描く。jsdom には canvas の描画が
// 無いので、描く命令を記録する文脈に差し替えて見る。

/** recorder は描く命令を順に記録し、getImageData では pixels を返す文脈である。 */
function recorder(pixels: number[] = []) {
  const calls: unknown[][] = [];
  const state: Pick<
    CanvasRenderingContext2D,
    | "globalCompositeOperation"
    | "strokeStyle"
    | "fillStyle"
    | "lineWidth"
    | "lineCap"
    | "lineJoin"
  > = {
    globalCompositeOperation: "source-over",
    strokeStyle: "",
    fillStyle: "",
    lineWidth: 1,
    lineCap: "butt",
    lineJoin: "miter",
  };
  const record =
    (name: string) =>
    (...args: unknown[]) => {
      calls.push([name, ...args, { ...state }]);
    };
  const image = { data: new Uint8ClampedArray(pixels) };
  const context = Object.assign(state, {
    beginPath: record("beginPath"),
    moveTo: record("moveTo"),
    lineTo: record("lineTo"),
    arc: record("arc"),
    fill: record("fill"),
    stroke: record("stroke"),
    fillRect: record("fillRect"),
    getImageData: () => image,
    putImageData: record("putImageData"),
  });
  return { context, calls, image };
}

const line: Stroke = {
  mode: "paint",
  size: 54,
  points: [
    { x: 576, y: 324 },
    { x: 1152, y: 648 },
  ],
};

describe("drawStroke", () => {
  it("線はコマの画素の点を結び、点 1 つは直径がブラシの丸にする", () => {
    const { context, calls } = recorder();
    drawStroke(context, line, "white", "black");
    expect(calls.map(([name]) => name)).toEqual([
      "beginPath",
      "moveTo",
      "lineTo",
      "stroke",
    ]);
    expect(calls[1]?.slice(1, 3)).toEqual([576, 324]);
    expect(calls[2]?.slice(1, 3)).toEqual([1152, 648]);
    expect(calls[3]?.[1]).toMatchObject({
      strokeStyle: "white",
      lineWidth: 54,
      lineCap: "round",
      globalCompositeOperation: "source-over",
    });

    const dot = recorder();
    drawStroke(
      dot.context,
      { mode: "paint", size: 20, points: [{ x: 5, y: 6 }] },
      "white",
      "black",
    );
    expect(dot.calls.map(([name]) => name)).toEqual(["beginPath", "arc", "fill"]);
    expect(dot.calls[1]?.slice(1, 4)).toEqual([5, 6, 10]);
  });

  it("消しはマスクでは黒で上塗りし、表示の層では destination-out で抜く", () => {
    const erase: Stroke = { ...line, mode: "erase" };
    const mask = recorder();
    drawStroke(mask.context, erase, "white", "black");
    expect(mask.calls.at(-1)?.[1]).toMatchObject({
      strokeStyle: "black",
      globalCompositeOperation: "source-over",
    });
    const layer = recorder();
    drawStroke(layer.context, erase, "white", null);
    expect(layer.calls.at(-1)?.[1]).toMatchObject({
      globalCompositeOperation: "destination-out",
    });
  });
});

describe("buildMask", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function stubCanvas(pixels: number[]) {
    const fake = recorder(pixels);
    const created: HTMLCanvasElement[] = [];
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(function (
      this: HTMLCanvasElement,
    ) {
      created.push(this);
      return fake.context as unknown as CanvasRenderingContext2D;
    } as unknown as HTMLCanvasElement["getContext"]);
    const toBlob = vi
      .spyOn(HTMLCanvasElement.prototype, "toBlob")
      .mockImplementation((callback, type) => callback(new Blob(["png"], { type })));
    return { ...fake, created, toBlob };
  }

  it("コマの大きさを黒で塗ってから筆跡を白で描き、中間の色を白か黒に寄せた PNG にする", async () => {
    // 2 画素: 縁のぼかしで半分より明るい画素と、暗い画素。
    const canvas = stubCanvas([200, 200, 200, 90, 40, 40, 40, 90]);
    const mask = await buildMask([line], 1920, 1080);

    expect(mask?.type).toBe("image/png");
    expect(canvas.created[0]?.width).toBe(1920);
    expect(canvas.created[0]?.height).toBe(1080);
    const names = canvas.calls.map(([name]) => name);
    expect(names[0]).toBe("fillRect");
    expect(canvas.calls[0]?.slice(1, 5)).toEqual([0, 0, 1920, 1080]);
    expect(canvas.calls[0]?.[5]).toMatchObject({ fillStyle: "black" });
    expect(canvas.calls.find(([name]) => name === "moveTo")?.slice(1, 3)).toEqual([
      576, 324,
    ]);
    expect(canvas.calls.find(([name]) => name === "stroke")?.[1]).toMatchObject({
      strokeStyle: "white",
    });
    expect([...canvas.image.data]).toEqual([255, 255, 255, 255, 0, 0, 0, 255]);
    expect(names).toContain("putImageData");
  });

  it("塗りが無いか、消しで塗りが全部消えたらマスクを送らない", async () => {
    stubCanvas([0, 0, 0, 255]);
    expect(await buildMask([], 1920, 1080)).toBeUndefined();
    expect(await buildMask([{ ...line, mode: "erase" }], 1920, 1080)).toBeUndefined();
    expect(
      await buildMask([line, { ...line, mode: "erase" }], 1920, 1080),
    ).toBeUndefined();
  });
});
