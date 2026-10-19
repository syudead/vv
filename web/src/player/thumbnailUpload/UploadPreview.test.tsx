import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UploadImage } from "./composeImage";
import { type Fit, WHOLE_IMAGE } from "./framing";
import UploadPreview from "./UploadPreview";

// jsdom cannot render canvas; verify the composition passed to the drawing boundary.

const compose = vi.hoisted(() => ({ drawComposition: vi.fn(), drawOverflow: vi.fn() }));
vi.mock("./composeImage", () => compose);

const image = {
  bitmap: { close: vi.fn() },
  width: 1000,
  height: 2000,
} as unknown as UploadImage;

function lastFit(): Fit {
  const call = compose.drawComposition.mock.calls.at(-1) as unknown[] | undefined;
  return call?.[3] as Fit;
}

function renderPreview(props: Partial<Parameters<typeof UploadPreview>[0]> = {}) {
  const onApply = vi.fn<(fit: Fit) => void>();
  const onCancel = vi.fn();
  render(
    <UploadPreview
      image={image}
      name="poster.png"
      aspect={16 / 9}
      narrow={false}
      applying={false}
      onApply={onApply}
      onCancel={onCancel}
      {...props}
    />,
  );
  return { onApply, onCancel };
}

function stage() {
  return screen.getByRole("img", { name: /Thumbnail preview/ });
}

function press(name: string) {
  fireEvent.click(screen.getByRole("button", { name }));
}

describe("UploadPreview", () => {
  beforeEach(() => {
    compose.drawComposition.mockReset();
    compose.drawOverflow.mockReset();
    // The 1024x576 preview is half the 2048x1152 output size.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 1024,
      bottom: 576,
      width: 1024,
      height: 576,
      toJSON: () => ({}),
    });
    HTMLElement.prototype.setPointerCapture = vi.fn();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("既定は全体を収め、枠の外にはみ出す分を縮めて別に描く", () => {
    renderPreview();
    expect(lastFit()).toEqual(WHOLE_IMAGE);
    expect(stage().getAttribute("tabindex")).toBe("0");
    expect(compose.drawOverflow).toHaveBeenLastCalledWith(
      expect.any(HTMLCanvasElement),
      image,
      16 / 9,
      WHOLE_IMAGE,
      0.25,
    );
    expect(
      screen.getByText(
        "Drag the image anywhere. Its edges and centre snap to the frame's.",
      ),
    ).toBeDefined();
  });

  it("引くとどの向きにも動き、中央の近くでは中央に吸い付き、離すと止まる", () => {
    renderPreview();
    fireEvent.pointerDown(stage(), { pointerId: 1, clientX: 500, clientY: 300 });
    // A 100px display drag moves 200 output pixels, 0.1 of the 2048px frame.
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 400, clientY: 400 });
    expect(lastFit().x).toBeCloseTo(-0.1);
    expect(lastFit().y).toBeCloseTo(200 / 1152);
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 495, clientY: 300 });
    expect(lastFit()).toMatchObject({ x: 0, y: 0 });
    const moved = lastFit();

    fireEvent.pointerMove(stage(), { pointerId: 2, clientX: 500, clientY: 0 });
    fireEvent.pointerUp(stage(), { pointerId: 1 });
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerDown(stage(), { pointerId: 3, clientX: 0, clientY: 0 });
    fireEvent.pointerCancel(stage(), { pointerId: 3 });
    fireEvent.pointerMove(stage(), { pointerId: 3, clientX: 0, clientY: 500 });
    expect(lastFit()).toEqual(moved);
  });

  it("キー・ホイール・つまみで位置と拡大率を変え、倍率を百分率で出す", () => {
    renderPreview();
    fireEvent.keyDown(stage(), { key: "=" });
    fireEvent.keyDown(stage(), { key: "+" });
    expect(lastFit().zoom).toBeCloseTo(1.5625);
    expect(screen.getByText("156%")).toBeDefined();
    fireEvent.keyDown(stage(), { key: "-" });
    expect(lastFit().zoom).toBeCloseTo(1.25);
    fireEvent.keyDown(stage(), { key: "ArrowRight" });
    expect(lastFit().x).toBeGreaterThan(0);
    fireEvent.keyDown(stage(), { key: "ArrowUp" });
    expect(lastFit().y).toBeLessThan(0);
    fireEvent.keyDown(stage(), { key: "a" });

    fireEvent.wheel(stage(), { deltaY: -100_000 });
    expect(lastFit().zoom).toBe(16);
    fireEvent.keyDown(screen.getByRole("slider", { name: "Zoom" }), { key: "Home" });
    expect(lastFit().zoom).toBe(0.25);
  });

  it("全体・枠を埋める・回転・余白の色を操作でき、全体に戻すと回転と余白は残る", () => {
    renderPreview();
    press("Fill frame");
    expect(lastFit().zoom).toBeCloseTo(2048 / 576);
    press("Turn right");
    expect(lastFit().rotation).toBe(90);
    press("Turn left");
    press("Turn left");
    expect(lastFit().rotation).toBe(270);
    fireEvent.click(screen.getByRole("radio", { name: "Black" }));
    expect(lastFit().backdrop).toBe("black");
    press("Whole image");
    expect(lastFit()).toEqual({ ...WHOLE_IMAGE, rotation: 270, backdrop: "black" });
  });

  it("適用は今の形を渡し、やめる・Esc は onCancel を呼ぶ", () => {
    const { onApply, onCancel } = renderPreview({
      initialFit: { ...WHOLE_IMAGE, x: 0.2 },
    });
    fireEvent.keyDown(stage(), { key: "+" });
    fireEvent.click(screen.getByRole("button", { name: "Use as thumbnail" }));
    expect(onApply).toHaveBeenCalledWith({ ...WHOLE_IMAGE, x: 0.25, zoom: 1.25 });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(2);
  });

  it("送る絵を作っている間は、適用も、やめるも、段の操作も効かない", () => {
    const { onApply, onCancel } = renderPreview({ applying: true });
    fireEvent.keyDown(stage(), { key: "+" });
    fireEvent.pointerDown(stage(), { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 300, clientY: 0 });
    expect(lastFit()).toEqual(WHOLE_IMAGE);
    expect(
      (screen.getByRole("button", { name: "Turn left" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    const apply = screen.getByRole("button", { name: "Use as thumbnail" });
    expect(apply.getAttribute("aria-busy")).toBe("true");
    fireEvent.click(apply);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    expect(onApply).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
    expect(
      (screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("動画の比率が分からなければ、枠は画像の形になると知らせる", () => {
    renderPreview({ aspect: null });
    expect(
      screen.getByText(
        "The video's size is unknown, so the frame keeps the image's shape.",
      ),
    ).toBeDefined();
    expect(compose.drawComposition.mock.calls.at(-1)?.[2]).toBeNull();
  });

  it("縦長の動画では、カードと同じく描いた絵をぼかした背景に写す", () => {
    const drawImage = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
    } as unknown as RenderingContext);
    compose.drawComposition.mockImplementation((canvas: HTMLCanvasElement) => {
      canvas.width = 1152;
      canvas.height = 2048;
    });
    renderPreview({ aspect: 9 / 16, narrow: true });
    const backdrop = screen
      .getByTestId("thumbnail-upload-preview")
      .querySelector("canvas[aria-hidden='true']");
    expect(backdrop).not.toBeNull();
    expect(drawImage).toHaveBeenCalledWith(expect.any(HTMLCanvasElement), 0, 0, 144, 256);
    act(() => press("Fill frame"));
    expect(drawImage).toHaveBeenCalledTimes(2);
  });
});
