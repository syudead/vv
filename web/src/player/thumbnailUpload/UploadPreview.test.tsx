import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UploadImage } from "./composeImage";
import type { Fit } from "./framing";
import UploadPreview from "./UploadPreview";

// プレビューの操作（収め方の切り替え、引く・ホイール・キー・つまみ）が、描く形をどう変えるか。
// jsdom には canvas の描画が無いので、描画は差し替えて、渡された形を見る。

const compose = vi.hoisted(() => ({ drawComposition: vi.fn() }));
vi.mock("./composeImage", () => compose);

const image = {
  bitmap: { close: vi.fn() },
  width: 1000,
  height: 2000,
} as unknown as UploadImage;

/** lastFit は最後に描いた形である。 */
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

function fillFrame() {
  fireEvent.click(screen.getByRole("radio", { name: "Fill frame" }));
}

describe("UploadPreview", () => {
  beforeEach(() => {
    compose.drawComposition.mockReset();
    // 段の canvas は 1024×576 に描かれ、送る画像（2048×1152）の半分の大きさで見える。
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

  it("既定は全体を収め、段は動かせず、拡大のつまみも無い", () => {
    renderPreview();
    expect(lastFit()).toEqual({ mode: "contain" });
    expect(stage().getAttribute("tabindex")).toBe("-1");
    expect(screen.queryByRole("slider", { name: "Zoom" })).toBeNull();
    expect(
      screen.getByText(
        "The whole image shows. The rest of the frame is filled with a blurred copy.",
      ),
    ).toBeDefined();
    fireEvent.keyDown(stage(), { key: "ArrowDown" });
    fireEvent.wheel(stage(), { deltaY: -500 });
    expect(lastFit()).toEqual({ mode: "contain" });
  });

  it("枠いっぱいに広げると、引いた分だけ画像が動き、離すと止まる", () => {
    renderPreview();
    fillFrame();
    expect(lastFit()).toEqual({ mode: "cover", zoom: 1, panX: 0, panY: 0 });
    expect(stage().getAttribute("tabindex")).toBe("0");

    fireEvent.pointerDown(stage(), { pointerId: 1, clientX: 500, clientY: 300 });
    // 画面で 100px 下へ引くと、送る画像では 200px 動く（動かせる幅は (4096-1152)/2）。
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 500, clientY: 400 });
    expect(lastFit()).toMatchObject({ panX: 0 });
    const moved = lastFit() as Extract<Fit, { mode: "cover" }>;
    expect(moved.panY).toBeCloseTo(-200 / 1472);

    // 別の指の動きと、離した後の動きは無視する。
    fireEvent.pointerMove(stage(), { pointerId: 2, clientX: 500, clientY: 0 });
    fireEvent.pointerUp(stage(), { pointerId: 1 });
    fireEvent.pointerMove(stage(), { pointerId: 1, clientX: 500, clientY: 0 });
    fireEvent.pointerDown(stage(), { pointerId: 3, clientX: 0, clientY: 0 });
    fireEvent.pointerCancel(stage(), { pointerId: 3 });
    fireEvent.pointerMove(stage(), { pointerId: 3, clientX: 0, clientY: 500 });
    expect(lastFit()).toEqual(moved);
  });

  it("キー・ホイール・つまみで拡大率と位置を変え、全体を収めるに戻すと初めの形になる", () => {
    renderPreview();
    fillFrame();
    fireEvent.keyDown(stage(), { key: "=" });
    fireEvent.keyDown(stage(), { key: "+" });
    expect(lastFit()).toMatchObject({ zoom: 1.5 });
    fireEvent.keyDown(stage(), { key: "-" });
    expect(lastFit()).toMatchObject({ zoom: 1.25 });
    fireEvent.keyDown(stage(), { key: "ArrowRight" });
    expect((lastFit() as { panX: number }).panX).toBeLessThan(0);
    fireEvent.keyDown(stage(), { key: "ArrowLeft" });
    fireEvent.keyDown(stage(), { key: "ArrowUp" });
    expect((lastFit() as { panY: number }).panY).toBeGreaterThan(0);
    fireEvent.keyDown(stage(), { key: "a" });

    fireEvent.wheel(stage(), { deltaY: -100_000 });
    expect(lastFit()).toMatchObject({ zoom: 4 });
    const slider = screen.getByRole("slider", { name: "Zoom" });
    fireEvent.keyDown(slider, { key: "Home" });
    expect(lastFit()).toMatchObject({ zoom: 1 });

    fireEvent.click(screen.getByRole("radio", { name: "Whole image" }));
    expect(lastFit()).toEqual({ mode: "contain" });
    fillFrame();
    expect(lastFit()).toEqual({ mode: "cover", zoom: 1, panX: 0, panY: 0 });
  });

  it("適用は今の形を渡し、やめる・Esc は onCancel を呼ぶ", () => {
    const { onApply, onCancel } = renderPreview();
    fillFrame();
    fireEvent.keyDown(stage(), { key: "+" });
    fireEvent.click(screen.getByRole("button", { name: "Use as thumbnail" }));
    expect(onApply).toHaveBeenCalledWith({ mode: "cover", zoom: 1.25, panX: 0, panY: 0 });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(2);
  });

  it("送る絵を作っている間は、適用も、やめるも、段の操作も効かない", () => {
    const { onApply, onCancel } = renderPreview({ applying: true });
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

  it("動画の比率が分からなければ、枠いっぱいに広げるは選べず、そのわけを出す", () => {
    renderPreview({ aspect: null });
    expect(
      (screen.getByRole("radio", { name: "Fill frame" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      screen.getByText("The video's size is unknown, so the image keeps its own shape."),
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
    act(() => fillFrame());
    expect(drawImage).toHaveBeenCalledTimes(2);
  });
});
