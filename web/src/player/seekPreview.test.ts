import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { attachSeekPreview, seekPreviewTarget } from "./seekPreview";

function pointer(
  type: string,
  clientX: number,
  pointerId = 1,
  pointerType = "mouse",
  clientY = 5,
) {
  const event = new Event(type) as PointerEvent;
  Object.defineProperties(event, {
    clientX: { value: clientX },
    clientY: { value: clientY },
    pointerId: { value: pointerId },
    pointerType: { value: pointerType },
  });
  return event;
}

function progressElement(): HTMLElement {
  const progress = document.createElement("div");
  progress.getBoundingClientRect = () =>
    ({
      left: 100,
      width: 400,
      right: 500,
      top: 0,
      bottom: 10,
      height: 10,
      x: 100,
      y: 0,
      toJSON: () => ({}),
    }) as DOMRect;
  progress.setPointerCapture = vi.fn();
  progress.releasePointerCapture = vi.fn();
  document.body.append(progress);
  return progress;
}

describe("seek preview target", () => {
  it("論理時刻を5秒bucketへ写し、両端ではpreviewを内側へ収める", () => {
    const rect = { left: 100, width: 400 };
    expect(seekPreviewTarget(100, rect, 120_000, 240)).toEqual({
      positionMs: 0,
      requestPositionMs: 0,
      leftPx: 120,
    });
    expect(seekPreviewTarget(301, rect, 120_000, 240)).toEqual({
      positionMs: 60_300,
      requestPositionMs: 60_000,
      leftPx: 201,
    });
    expect(seekPreviewTarget(303, rect, 120_000, 240).requestPositionMs).toBe(60_000);
    expect(seekPreviewTarget(500, rect, 120_000, 240)).toEqual({
      positionMs: 119_999,
      requestPositionMs: 115_000,
      leftPx: 280,
    });
  });
});

describe("seek preview controller", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    document.body.replaceChildren();
  });

  it("時刻を即時表示し、debounce後に現在bucketの画像だけを表示する", async () => {
    const progress = progressElement();
    const requests: Array<{
      url: string;
      signal: AbortSignal;
      resolve: (blob: Blob) => void;
    }> = [];
    const fetchImage = vi.fn(
      (url: string, signal: AbortSignal) =>
        new Promise<Blob>((resolve) => requests.push({ url, signal, resolve })),
    );
    const detach = attachSeekPreview(progress, {
      durationMs: 120_000,
      thumbnailUrl: "/api/videos/1/seek-thumbnail?v=content",
      fetchImage,
    });
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    const image = progress.querySelector<HTMLImageElement>("img");
    if (preview === null || image === null) throw new Error("preview DOMがありません");
    Object.defineProperty(preview, "offsetWidth", { value: 240 });

    progress.dispatchEvent(pointer("pointerenter", 300));
    expect(preview.dataset.state).toBe("loading");
    expect(preview.textContent).toBe("1:00");
    expect(fetchImage).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(150);
    expect(requests[0]?.url).toBe(
      "/api/videos/1/seek-thumbnail?v=content&positionMs=60000",
    );

    progress.dispatchEvent(pointer("pointermove", 400));
    expect(requests[0]?.signal.aborted).toBe(true);
    expect(preview.dataset.state).toBe("loading");
    expect(image.hasAttribute("src")).toBe(false);
    await vi.advanceTimersByTimeAsync(150);

    requests[0]?.resolve(new Blob(["old"]));
    requests[1]?.resolve(new Blob(["current"]));
    await Promise.resolve();
    await Promise.resolve();
    expect(preview.dataset.state).toBe("ready");
    expect(image.src).toContain("positionMs=90000");

    progress.dispatchEvent(pointer("pointerleave", 400));
    expect(preview.dataset.state).toBe("hidden");
    detach();
    expect(progress.querySelector(".vv-seek-preview")).toBeNull();
  });

  it("細いbarではなくprogress control全体をhover領域にする", () => {
    const progress = progressElement();
    const interactionTarget = document.createElement("div");
    interactionTarget.getBoundingClientRect = () =>
      ({
        left: 90,
        width: 420,
        right: 510,
        top: -20,
        bottom: 30,
        height: 50,
        x: 90,
        y: -20,
        toJSON: () => ({}),
      }) as DOMRect;
    interactionTarget.setPointerCapture = vi.fn();
    interactionTarget.releasePointerCapture = vi.fn();
    document.body.append(interactionTarget);

    attachSeekPreview(
      progress,
      {
        durationMs: 120_000,
        thumbnailUrl: "/preview",
        fetchImage: vi.fn(() => new Promise<Blob>(() => undefined)),
      },
      interactionTarget,
    );
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    if (preview === null) throw new Error("preview DOMがありません");

    interactionTarget.dispatchEvent(pointer("pointerenter", 300, 1, "mouse", -10));
    expect(preview.dataset.state).toBe("loading");
    expect(preview.textContent).toBe("1:00");
  });

  it("touch dragはbar外でも追従し、終了時に隠す", async () => {
    const progress = progressElement();
    const fetchImage = vi.fn(() => Promise.reject(new Error("unavailable")));
    attachSeekPreview(progress, {
      durationMs: 10_000,
      thumbnailUrl: "/preview",
      fetchImage,
    });
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    if (preview === null) throw new Error("preview DOMがありません");
    let normalWidth = 160;
    Object.defineProperty(preview, "offsetWidth", {
      get: () => (preview.dataset.state === "unavailable" ? 30 : normalWidth),
    });

    progress.dispatchEvent(pointer("pointerdown", 250, 7, "touch"));
    progress.dispatchEvent(pointer("pointermove", 900, 7, "touch"));
    expect(progress.setPointerCapture).toHaveBeenCalledWith(7);
    expect(preview.textContent).toBe("0:09");
    await vi.advanceTimersByTimeAsync(150);
    await Promise.resolve();
    await Promise.resolve();
    expect(preview.dataset.state).toBe("unavailable");

    normalWidth = 240;
    progress.dispatchEvent(pointer("pointermove", 100, 7, "touch"));
    expect(preview.dataset.state).toBe("loading");
    expect(preview.style.left).toBe("120px");

    progress.dispatchEvent(pointer("pointerup", 900, 7, "touch"));
    expect(preview.dataset.state).toBe("hidden");
  });

  it("mouse dragをbar内で終えるとhover previewを維持し、bar外では隠す", () => {
    const progress = progressElement();
    attachSeekPreview(progress, {
      durationMs: 10_000,
      thumbnailUrl: "/preview",
      fetchImage: vi.fn(() => new Promise<Blob>(() => undefined)),
    });
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    if (preview === null) throw new Error("preview DOMがありません");
    Object.defineProperty(preview, "offsetWidth", { value: 160 });

    progress.dispatchEvent(pointer("pointerdown", 250));
    progress.dispatchEvent(pointer("pointerup", 250));
    expect(preview.dataset.state).toBe("loading");

    progress.dispatchEvent(pointer("pointerdown", 250));
    progress.dispatchEvent(pointer("pointerup", 600));
    expect(preview.dataset.state).toBe("hidden");
  });
});
