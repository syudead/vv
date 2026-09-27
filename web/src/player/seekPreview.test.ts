import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SeekThumbnailSprite } from "../api/client";
import { attachSeekPreview, seekPreviewTarget, seekSpriteCell } from "./seekPreview";

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

const twoHoursMs = 7_200_000;

// 2 時間の動画の配置情報（12 秒間隔・600 コマ・6 シート）。
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

interface Deferred<T> {
  url: string;
  signal: AbortSignal;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
}

// fakeFetchers は配置情報とシートの取得を控え、テストが好きな時に終わらせられるようにする。
function fakeFetchers() {
  const sprites: Array<Deferred<SeekThumbnailSprite>> = [];
  const sheets: Array<Deferred<Blob>> = [];
  const fetchSprite = vi.fn(
    (url: string, signal: AbortSignal) =>
      new Promise<SeekThumbnailSprite>((resolve, reject) =>
        sprites.push({ url, signal, resolve, reject }),
      ),
  );
  const fetchSheet = vi.fn(
    (url: string, signal: AbortSignal) =>
      new Promise<Blob>((resolve, reject) =>
        sheets.push({ url, signal, resolve, reject }),
      ),
  );
  return { sprites, sheets, fetchSprite, fetchSheet };
}

const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

function previewParts(progress: HTMLElement) {
  const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
  const image = progress.querySelector<HTMLElement>(".vv-seek-preview-image");
  if (preview === null || image === null) throw new Error("preview DOMがありません");
  Object.defineProperty(preview, "offsetWidth", { value: 240 });
  return { preview, image };
}

let objectUrls = 0;
const createObjectURL = vi.fn(() => `blob:sheet-${String(++objectUrls)}`);
const revokeObjectURL = vi.fn();

beforeEach(() => {
  objectUrls = 0;
  createObjectURL.mockClear();
  revokeObjectURL.mockClear();
  Object.defineProperty(URL, "createObjectURL", {
    value: createObjectURL,
    configurable: true,
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    value: revokeObjectURL,
    configurable: true,
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe("seek preview target", () => {
  it("論理時刻を求め、両端ではpreviewを内側へ収める", () => {
    const rect = { left: 100, width: 400 };
    expect(seekPreviewTarget(100, rect, 120_000, 240)).toEqual({
      positionMs: 0,
      leftPx: 120,
    });
    expect(seekPreviewTarget(301, rect, 120_000, 240)).toEqual({
      positionMs: 60_300,
      leftPx: 201,
    });
    expect(seekPreviewTarget(500, rect, 120_000, 240)).toEqual({
      positionMs: 119_999,
      leftPx: 280,
    });
  });
});

describe("seek sprite cell", () => {
  it("配置情報の間隔でコマを決め、5秒を前提にしない", () => {
    const sprite = twoHourSprite();
    expect(seekSpriteCell(sprite, 0)).toEqual({ frame: 0, sheet: 0, column: 0, row: 0 });
    // 5 秒間隔なら 1 コマ目だが、12 秒間隔では 0 コマ目のまま。
    expect(seekSpriteCell(sprite, 11_999).frame).toBe(0);
    expect(seekSpriteCell(sprite, 12_000)).toEqual({
      frame: 1,
      sheet: 0,
      column: 1,
      row: 0,
    });
    expect(seekSpriteCell(sprite, 3_618_000)).toEqual({
      frame: 301,
      sheet: 3,
      column: 1,
      row: 0,
    });
    expect(seekSpriteCell(sprite, 5_400_000)).toEqual({
      frame: 450,
      sheet: 4,
      column: 0,
      row: 5,
    });
    // 末尾を越える位置は最後のコマに寄せる。
    expect(seekSpriteCell(sprite, twoHoursMs + 60_000)).toEqual({
      frame: 599,
      sheet: 5,
      column: 9,
      row: 9,
    });
    expect(
      seekSpriteCell({ ...sprite, intervalMs: 5000, frameCount: 24 }, 60_000).frame,
    ).toBe(12);
  });
});

describe("seek preview controller", () => {
  it("最初の表示で配置情報を取り、同じシートの中の移動では取得しない", async () => {
    const progress = progressElement();
    const fetchers = fakeFetchers();
    attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/api/videos/1/seek-thumbnail?v=c",
      ...fetchers,
    });
    const { preview, image } = previewParts(progress);

    progress.dispatchEvent(pointer("pointerenter", 300));
    expect(preview.dataset.state).toBe("loading");
    expect(preview.textContent).toBe("1:00:00");
    expect(fetchers.fetchSprite).toHaveBeenCalledTimes(1);
    expect(fetchers.sprites[0]?.url).toBe("/api/videos/1/seek-thumbnail?v=c");

    fetchers.sprites[0]?.resolve(twoHourSprite());
    await flush();
    expect(preview.style.getPropertyValue("--vv-seek-frame-aspect")).toBe("320 / 180");
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(1);
    expect(fetchers.sheets[0]?.url).toBe("/api/videos/1/seek-thumbnail/3?v=c");

    fetchers.sheets[0]?.resolve(new Blob(["sheet-3"]));
    await flush();
    expect(preview.dataset.state).toBe("ready");
    expect(image.style.backgroundImage).toContain("blob:sheet-1");
    expect(image.style.backgroundSize).toBe("1000% 1000%");
    expect(image.style.backgroundPosition).toBe("0% 0%");

    // 同じシートの隣のコマ（301）へ。取得は起きず、1 列ずれる。
    progress.dispatchEvent(pointer("pointermove", 301));
    expect(preview.dataset.state).toBe("ready");
    expect(image.style.backgroundPosition).toMatch(/^11\.1+\d*% 0%$/);
    expect(fetchers.fetchSprite).toHaveBeenCalledTimes(1);
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(1);

    // 隠して出し直しても、配置情報もシートも取り直さない。
    progress.dispatchEvent(pointer("pointerleave", 301));
    expect(preview.dataset.state).toBe("hidden");
    progress.dispatchEvent(pointer("pointerenter", 300));
    expect(preview.dataset.state).toBe("ready");
    expect(fetchers.fetchSprite).toHaveBeenCalledTimes(1);
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(1);
  });

  it("シートをまたぐと次のシートを1回だけ取り、取得中に戻っても2回目を出さない", async () => {
    const progress = progressElement();
    const fetchers = fakeFetchers();
    attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fetchers,
    });
    const { preview, image } = previewParts(progress);

    progress.dispatchEvent(pointer("pointerenter", 300));
    fetchers.sprites[0]?.resolve(twoHourSprite());
    await flush();
    expect(fetchers.sheets.map((s) => s.url)).toEqual([
      "/api/videos/1/seek-thumbnail/3?v=c",
    ]);

    // シート 3 の取得が終わる前にシート 4 へ。シート 3 の取得は中断しない。
    progress.dispatchEvent(pointer("pointermove", 400));
    progress.dispatchEvent(pointer("pointermove", 401));
    expect(preview.dataset.state).toBe("loading");
    expect(fetchers.sheets.map((s) => s.url)).toEqual([
      "/api/videos/1/seek-thumbnail/3?v=c",
      "/api/videos/1/seek-thumbnail/4?v=c",
    ]);
    expect(fetchers.sheets[0]?.signal.aborted).toBe(false);

    // 表示中でないシート 3 の取得が終わっても、表示は切り替えない。
    fetchers.sheets[0]?.resolve(new Blob(["sheet-3"]));
    await flush();
    expect(preview.dataset.state).toBe("loading");
    expect(image.style.backgroundImage).toBe("");

    // シート 3 へ戻れば、取り直さずに出す。
    progress.dispatchEvent(pointer("pointermove", 300));
    expect(preview.dataset.state).toBe("ready");
    expect(image.style.backgroundImage).toContain("blob:sheet-1");

    // 取得中のシート 4 へ戻っても、2 回目の取得は起きず、完了を待って出す。
    progress.dispatchEvent(pointer("pointermove", 400));
    expect(preview.dataset.state).toBe("loading");
    fetchers.sheets[1]?.resolve(new Blob(["sheet-4"]));
    await flush();
    expect(preview.dataset.state).toBe("ready");
    expect(image.style.backgroundImage).toContain("blob:sheet-2");
    expect(image.style.backgroundPosition).toBe("0% 55.55555555555556%");
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(2);
  });

  it("配置情報の取得に失敗しても時刻を出し、5秒後に取り直す", async () => {
    let now = 10_000;
    vi.spyOn(Date, "now").mockImplementation(() => now);
    const progress = progressElement();
    const fetchers = fakeFetchers();
    attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fetchers,
    });
    const { preview } = previewParts(progress);

    progress.dispatchEvent(pointer("pointerenter", 300));
    fetchers.sprites[0]?.reject(new Error("generating"));
    await flush();
    expect(preview.dataset.state).toBe("unavailable");
    expect(preview.textContent).toBe("1:00:00");

    progress.dispatchEvent(pointer("pointermove", 400));
    expect(preview.dataset.state).toBe("unavailable");
    expect(preview.textContent).toBe("1:30:00");
    expect(fetchers.fetchSprite).toHaveBeenCalledTimes(1);

    now += 5000;
    progress.dispatchEvent(pointer("pointermove", 400));
    expect(preview.dataset.state).toBe("loading");
    expect(fetchers.fetchSprite).toHaveBeenCalledTimes(2);
  });

  it("シートの取得に失敗しても時刻を出し、そのシートだけ5秒後に取り直す", async () => {
    let now = 10_000;
    vi.spyOn(Date, "now").mockImplementation(() => now);
    const progress = progressElement();
    const fetchers = fakeFetchers();
    attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fetchers,
    });
    const { preview } = previewParts(progress);

    progress.dispatchEvent(pointer("pointerenter", 300));
    fetchers.sprites[0]?.resolve(twoHourSprite());
    await flush();
    fetchers.sheets[0]?.reject(new Error("read failed"));
    await flush();
    expect(preview.dataset.state).toBe("unavailable");
    expect(preview.textContent).toBe("1:00:00");

    progress.dispatchEvent(pointer("pointermove", 301));
    expect(preview.dataset.state).toBe("unavailable");
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(1);

    // 別のシートは待たずに取る。
    progress.dispatchEvent(pointer("pointermove", 400));
    expect(preview.dataset.state).toBe("loading");
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(2);

    now += 5000;
    progress.dispatchEvent(pointer("pointermove", 300));
    expect(preview.dataset.state).toBe("loading");
    expect(fetchers.fetchSheet).toHaveBeenCalledTimes(3);
    expect(fetchers.sheets[2]?.url).toBe("/api/videos/1/seek-thumbnail/3?v=c");
  });

  it("取り外しで進行中の取得を中断し、object URLを解放する", async () => {
    const progress = progressElement();
    const fetchers = fakeFetchers();
    const detach = attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fetchers,
    });
    previewParts(progress);

    progress.dispatchEvent(pointer("pointerenter", 300));
    fetchers.sprites[0]?.resolve(twoHourSprite());
    await flush();
    fetchers.sheets[0]?.resolve(new Blob(["sheet-3"]));
    await flush();
    progress.dispatchEvent(pointer("pointermove", 400));
    expect(fetchers.sheets[1]?.signal.aborted).toBe(false);

    detach();
    expect(fetchers.sheets[1]?.signal.aborted).toBe(true);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");
    expect(progress.querySelector(".vv-seek-preview")).toBeNull();

    // 中断した取得の後始末で object URL を作らない。
    fetchers.sheets[1]?.resolve(new Blob(["late"]));
    await flush();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
  });

  it("配置情報の取得中に取り外すと、その取得を中断する", () => {
    const progress = progressElement();
    const fetchers = fakeFetchers();
    const detach = attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fetchers,
    });
    progress.dispatchEvent(pointer("pointerenter", 300));
    detach();
    expect(fetchers.sprites[0]?.signal.aborted).toBe(true);
  });

  it("プレビューは読み上げの対象にしない", () => {
    const progress = progressElement();
    attachSeekPreview(progress, {
      durationMs: twoHoursMs,
      thumbnailUrl: "/sprite",
      ...fakeFetchers(),
    });
    expect(progress.querySelector(".vv-seek-preview")?.getAttribute("aria-hidden")).toBe(
      "true",
    );
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
      { durationMs: 120_000, thumbnailUrl: "/sprite", ...fakeFetchers() },
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
    const fetchSprite = vi.fn(() => Promise.reject(new Error("unavailable")));
    attachSeekPreview(progress, {
      durationMs: 10_000,
      thumbnailUrl: "/sprite",
      fetchSprite,
      fetchSheet: vi.fn(),
    });
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    if (preview === null) throw new Error("preview DOMがありません");
    Object.defineProperty(preview, "offsetWidth", {
      get: () => (preview.dataset.state === "unavailable" ? 30 : 240),
    });

    progress.dispatchEvent(pointer("pointerdown", 250, 7, "touch"));
    progress.dispatchEvent(pointer("pointermove", 900, 7, "touch"));
    expect(progress.setPointerCapture).toHaveBeenCalledWith(7);
    expect(preview.textContent).toBe("0:09");
    await flush();
    expect(preview.dataset.state).toBe("unavailable");

    progress.dispatchEvent(pointer("pointermove", 100, 7, "touch"));
    expect(preview.dataset.state).toBe("unavailable");
    expect(preview.style.left).toBe("15px");
    expect(preview.textContent).toBe("0:00");

    progress.dispatchEvent(pointer("pointerup", 900, 7, "touch"));
    expect(preview.dataset.state).toBe("hidden");
  });

  it("mouse dragをbar内で終えるとhover previewを維持し、bar外では隠す", () => {
    const progress = progressElement();
    attachSeekPreview(progress, {
      durationMs: 10_000,
      thumbnailUrl: "/sprite",
      ...fakeFetchers(),
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
