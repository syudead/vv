import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
  type Video,
} from "../api/client";
import { ScrubBand, ScrubFrame, useScrubPreview } from "./ScrubPreview";

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  fetchSeekThumbnailSprite: vi.fn(),
  fetchSeekThumbnailSheet: vi.fn(),
}));

const twoHoursMs = 7_200_000;

function video(extra: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "動画 1",
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: "/thumbnail.jpg",
    durationMs: twoHoursMs,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "pending",
    seekThumbnailUrl: "/api/videos/1/seek-thumbnail?v=c",
    tags: [],
    ...extra,
  };
}

// 2 時間の動画の配置情報（12 秒間隔・600 コマ・10×10 の 6 シート）。帯の左半分は
// シート 0〜2、真ん中から右はシート 3〜5 になる。
function twoHourSprite(version = "c"): SeekThumbnailSprite {
  return {
    intervalMs: 12_000,
    frameCount: 600,
    columns: 10,
    rows: 10,
    frameWidth: 320,
    frameHeight: 180,
    sheets: [0, 1, 2, 3, 4, 5].map(
      (n) => `/api/videos/1/seek-thumbnail/${String(n)}?v=${version}`,
    ),
  };
}

interface Deferred<T> {
  url: string;
  signal: AbortSignal;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
}

let sprites: Array<Deferred<SeekThumbnailSprite>> = [];
let sheets: Array<Deferred<Blob>> = [];
const order: string[] = [];

let objectUrls = 0;
const createObjectURL = vi.fn(() => `blob:sheet-${String(++objectUrls)}`);
const revokeObjectURL = vi.fn();

let intersect: ((visible: boolean) => void) | undefined;

beforeEach(() => {
  sprites = [];
  sheets = [];
  order.length = 0;
  objectUrls = 0;
  createObjectURL.mockClear();
  revokeObjectURL.mockClear();
  vi.mocked(fetchSeekThumbnailSprite).mockImplementation(
    (url, signal) =>
      new Promise((resolve, reject) => {
        order.push(url);
        sprites.push({ url, signal, resolve, reject });
      }),
  );
  vi.mocked(fetchSeekThumbnailSheet).mockImplementation(
    (url, signal) =>
      new Promise((resolve, reject) => {
        order.push(url);
        sheets.push({ url, signal, resolve, reject });
      }),
  );
  Object.defineProperty(URL, "createObjectURL", {
    value: createObjectURL,
    configurable: true,
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    value: revokeObjectURL,
    configurable: true,
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left: 100,
    width: 400,
    right: 500,
    top: 200,
    bottom: 220,
    height: 20,
    x: 100,
    y: 200,
    toJSON: () => ({}),
  });
  intersect = undefined;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = (visible) =>
          callback(
            [{ isIntersecting: visible } as IntersectionObserverEntry],
            this as unknown as IntersectionObserver,
          );
      }
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  vi.mocked(fetchSeekThumbnailSprite).mockReset();
  vi.mocked(fetchSeekThumbnailSheet).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function Harness({
  item = video(),
  selectionMode = false,
  onSuspend,
  onResume,
}: {
  item?: Video;
  selectionMode?: boolean;
  onSuspend?: () => void;
  onResume?: () => void;
}) {
  const scrub = useScrubPreview({ video: item, selectionMode, onSuspend, onResume });
  return (
    <div data-testid="card" ref={scrub.cardRef} onPointerLeave={scrub.leaveCard}>
      <span data-testid="above">thumbnail</span>
      <ScrubFrame frame={scrub.frame} />
      <span data-testid="position">{scrub.position?.positionMs ?? "none"}</span>
      <ScrubBand scrub={scrub} />
    </div>
  );
}

function parts() {
  const card = screen.getByTestId("card");
  const band = card.querySelector<HTMLElement>("[data-scrub-band]");
  if (band === null) throw new Error("帯がありません");
  return { card, band, above: screen.getByTestId("above") };
}

// 帯の位置（clientX）は 100〜500。真ん中の 300 から右はシート 3 以降。
// React は React の要素どうしの移動を、出た側の pointerout から enter/leave にする。
function enterBand(clientX: number, pointerType = "mouse") {
  const { band, above } = parts();
  fireEvent.pointerOut(above, { pointerType, clientX, relatedTarget: band });
}

function moveBand(clientX: number, pointerType = "mouse") {
  fireEvent.pointerMove(parts().band, { pointerType, clientX });
}

/** 帯から上へ出て、カードの中に留まる。 */
function leaveBandToCard() {
  const { band, above } = parts();
  fireEvent.pointerOut(band, { pointerType: "mouse", relatedTarget: above });
}

/** カードの外へ出る。 */
function leaveCard() {
  fireEvent.pointerOut(parts().band, {
    pointerType: "mouse",
    relatedTarget: document.body,
  });
}

/** カードの外から入り直す（帯には入らない）。 */
function enterCard() {
  fireEvent.pointerOver(parts().above, {
    pointerType: "mouse",
    relatedTarget: document.body,
  });
}

function frameImage(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-scrub-frame-image]");
}

async function settle<T>(deferred: Deferred<T> | undefined, value: T) {
  if (deferred === undefined) throw new Error("取得がありません");
  await act(async () => {
    deferred.resolve(value);
    await Promise.resolve();
  });
}

async function fail<T>(deferred: Deferred<T> | undefined) {
  if (deferred === undefined) throw new Error("取得がありません");
  await act(async () => {
    deferred.reject(new Error("409"));
    await Promise.resolve();
  });
}

const sheet = () => new Blob(["jpeg"], { type: "image/jpeg" });

describe("useScrubPreview", () => {
  it("帯に入るまで取得せず、初回の進入で配置情報→そのコマのシートを1回ずつ取る", async () => {
    render(<Harness />);
    enterCard();
    fireEvent.pointerMove(parts().above, { pointerType: "mouse", clientX: 150 });
    expect(fetchSeekThumbnailSprite).not.toHaveBeenCalled();

    enterBand(110);
    expect(screen.getByTestId("position").textContent).toBe("180000");
    expect(sprites).toHaveLength(1);
    expect(sprites[0]?.url).toBe("/api/videos/1/seek-thumbnail?v=c");
    expect(sheets).toHaveLength(0);
    // 取得待ちの間はコマの要素を出さない。
    expect(document.querySelector("[data-scrub-frame]")).toBeNull();

    await settle(sprites[0], twoHourSprite());
    expect(sheets).toHaveLength(1);
    expect(order).toEqual([
      "/api/videos/1/seek-thumbnail?v=c",
      "/api/videos/1/seek-thumbnail/0?v=c",
    ]);
    expect(frameImage()).toBeNull();

    await settle(sheets[0], sheet());
    const image = frameImage();
    expect(image?.style.backgroundImage).toBe('url("blob:sheet-1")');
    // 180_000ms = 15 コマ目（列 5・行 1）。
    expect(image?.style.backgroundPosition).toBe(
      `${String((5 / 9) * 100)}% ${String((1 / 9) * 100)}%`,
    );
    expect(image?.style.backgroundSize).toBe("1000% 1000%");
    expect(
      document.querySelector("[data-scrub-frame]")?.getAttribute("aria-hidden"),
    ).toBe("true");
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(1);
  });

  it("帯を出入りしても取り直さず、帯の外では表示を消す", async () => {
    const onSuspend = vi.fn();
    const onResume = vi.fn();
    render(<Harness onSuspend={onSuspend} onResume={onResume} />);
    enterBand(110);
    expect(onSuspend).toHaveBeenCalledTimes(1);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    expect(frameImage()).not.toBeNull();

    leaveBandToCard();
    expect(onResume).toHaveBeenCalledTimes(1);
    expect(frameImage()).toBeNull();
    expect(screen.getByTestId("position").textContent).toBe("none");

    enterBand(120);
    expect(onSuspend).toHaveBeenCalledTimes(2);
    expect(frameImage()).not.toBeNull();

    leaveCard();
    enterCard();
    enterBand(130);
    expect(frameImage()).not.toBeNull();
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(1);
  });

  it("シートをまたぐ位置では、次のシートだけを取る", async () => {
    render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());

    moveBand(300);
    expect(sheets.map((item) => item.url)).toEqual([
      "/api/videos/1/seek-thumbnail/0?v=c",
      "/api/videos/1/seek-thumbnail/3?v=c",
    ]);
    // 次のシートが届くまではコマを出さない。
    expect(frameImage()).toBeNull();
    // 取得中に同じシートの位置を動いても、2 回目は出さない。
    moveBand(310);
    expect(sheets).toHaveLength(2);

    await settle(sheets[1], sheet());
    expect(frameImage()?.style.backgroundImage).toBe('url("blob:sheet-2")');

    // 持っているシートへ戻っても取り直さない。
    moveBand(110);
    expect(frameImage()?.style.backgroundImage).toBe('url("blob:sheet-1")');
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(2);
  });

  it("取得が終わった時点の最新の位置のコマから出す", async () => {
    render(<Harness />);
    enterBand(110);
    moveBand(120);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    // 120 → 360_000ms = 30 コマ目（列 0・行 3）。
    expect(screen.getByTestId("position").textContent).toBe("360000");
    expect(frameImage()?.style.backgroundPosition).toBe(`0% ${String((3 / 9) * 100)}%`);
  });

  it("取得が終わる前に帯を出たらコマを出さず、カードの中にいる間は取得を続ける", async () => {
    render(<Harness />);
    enterBand(110);
    leaveBandToCard();
    expect(sprites[0]?.signal.aborted).toBe(false);

    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    expect(frameImage()).toBeNull();

    enterBand(110);
    expect(frameImage()).not.toBeNull();
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);
  });

  it("配置情報の取得中にカードを出たら中断し、入り直して帯へ入ると取り直してコマを出す", async () => {
    render(<Harness />);
    enterBand(110);
    leaveCard();
    expect(sprites[0]?.signal.aborted).toBe(true);
    // 中断した取得が後から終わっても使わない。
    await settle(sprites[0], twoHourSprite());
    expect(sheets).toHaveLength(0);

    enterCard();
    enterBand(110);
    expect(sprites).toHaveLength(2);
    await settle(sprites[1], twoHourSprite());
    await settle(sheets[0], sheet());
    expect(frameImage()).not.toBeNull();
  });

  it("シートの取得中にカードを出たら中断し、入り直して帯へ入ると取り直してコマを出す", async () => {
    render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    leaveCard();
    expect(sheets[0]?.signal.aborted).toBe(true);

    enterCard();
    enterBand(110);
    // 配置情報は持っているので、取り直すのはシートだけ。
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);
    expect(sheets).toHaveLength(2);
    await settle(sheets[1], sheet());
    expect(frameImage()).not.toBeNull();
  });

  it("失敗したら何も出さず、カードを出て入り直すまで取り直さない", async () => {
    render(<Harness />);
    enterBand(110);
    await fail(sprites[0]);
    expect(document.querySelector("[data-scrub-frame]")).toBeNull();
    // 時刻とバーのための位置は失敗に関わらず帯にいる間は持つ。
    expect(screen.getByTestId("position").textContent).toBe("180000");

    moveBand(300);
    leaveBandToCard();
    enterBand(110);
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);

    leaveCard();
    enterCard();
    enterBand(110);
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(2);
    await settle(sprites[1], twoHourSprite());
    await fail(sheets[0]);
    moveBand(120);
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(1);
    expect(document.querySelector("[data-scrub-frame]")).toBeNull();

    leaveCard();
    enterCard();
    enterBand(110);
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(2);
  });

  it("viewportから外れたらobject URLを解放し、次の進入で取り直す", async () => {
    render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    moveBand(300);
    await settle(sheets[1], sheet());
    leaveCard();

    act(() => intersect?.(true));
    expect(revokeObjectURL).not.toHaveBeenCalled();
    act(() => intersect?.(false));
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-2");

    enterCard();
    enterBand(110);
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(2);
    await settle(sprites[1], twoHourSprite());
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(3);
  });

  it("unmountでobject URLを解放し、進行中の取得を中断する", async () => {
    const { unmount } = render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    moveBand(300);
    unmount();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");
    expect(sheets[1]?.signal.aborted).toBe(true);
  });

  it("touchとpenでは何も起きない", () => {
    const onSuspend = vi.fn();
    render(<Harness onSuspend={onSuspend} />);
    for (const pointerType of ["touch", "pen"]) {
      enterBand(110, pointerType);
      moveBand(300, pointerType);
    }
    expect(onSuspend).not.toHaveBeenCalled();
    expect(fetchSeekThumbnailSprite).not.toHaveBeenCalled();
    expect(screen.getByTestId("position").textContent).toBe("none");
  });

  it("スプライトの無い動画・長さの無い動画・再生できない動画には帯を置かない", () => {
    const { rerender } = render(
      <Harness item={video({ seekThumbnailUrl: undefined })} />,
    );
    expect(document.querySelector("[data-scrub-band]")).toBeNull();
    rerender(<Harness item={video({ durationMs: 0 })} />);
    expect(document.querySelector("[data-scrub-band]")).toBeNull();
    rerender(<Harness item={video({ playable: false, probeState: "failed" })} />);
    expect(document.querySelector("[data-scrub-band]")).toBeNull();
    // ループのプレビューが無くても、スプライトがあれば帯は働く。
    rerender(<Harness item={video({ previewState: "failed" })} />);
    const band = document.querySelector("[data-scrub-band]");
    expect(band).not.toBeNull();
    expect(band?.getAttribute("aria-hidden")).toBe("true");
    expect(band?.hasAttribute("tabindex")).toBe(false);
  });

  it("選択モードでは帯を置かない", () => {
    function Selecting() {
      const scrub = useScrubPreview({ video: video(), selectionMode: true });
      return <ScrubBand scrub={scrub} />;
    }
    render(<Selecting />);
    expect(document.querySelector("[data-scrub-band]")).toBeNull();
  });

  it("取得中に帯が無効になったら中断し、後から終わってもシートを取らない", async () => {
    const { rerender } = render(<Harness />);
    enterBand(200);
    rerender(<Harness selectionMode />);
    expect(document.querySelector("[data-scrub-band]")).toBeNull();
    expect(sprites[0]?.signal.aborted).toBe(true);

    await settle(sprites[0], twoHourSprite());
    expect(sheets).toHaveLength(0);
    expect(fetchSeekThumbnailSheet).not.toHaveBeenCalled();
  });

  it("帯にいる間にスプライトのURLが変わったら、ポインタが動かなくても新しいURLから取り直す", async () => {
    const { rerender } = render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());
    expect(frameImage()?.style.backgroundImage).toBe('url("blob:sheet-1")');
    // 新しい URL の取得が始まる前に、古いシートの取得を待つ状態にしておく。
    moveBand(300);
    const stale = sheets[1];

    rerender(
      <Harness item={video({ seekThumbnailUrl: "/api/videos/1/seek-thumbnail?v=d" })} />,
    );
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");
    expect(stale?.signal.aborted).toBe(true);
    expect(screen.getByTestId("position").textContent).not.toBe("none");
    expect(sprites).toHaveLength(2);
    expect(sprites[1]?.url).toBe("/api/videos/1/seek-thumbnail?v=d");

    // 古い取得が後から終わっても、新しい URL のシートに混ざらない。
    await settle(stale, sheet());
    expect(frameImage()).toBeNull();

    await settle(sprites[1], twoHourSprite("d"));
    expect(sheets[2]?.url).toBe("/api/videos/1/seek-thumbnail/3?v=d");
    await settle(sheets[2], sheet());
    expect(frameImage()?.style.backgroundImage).toBe('url("blob:sheet-2")');
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(2);
  });

  it("viewportから外れて解放した後にURLが変わっても取り直さない", async () => {
    const { rerender } = render(<Harness />);
    enterBand(110);
    await settle(sprites[0], twoHourSprite());
    await settle(sheets[0], sheet());

    act(() => intersect?.(false));
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:sheet-1");

    rerender(
      <Harness item={video({ seekThumbnailUrl: "/api/videos/1/seek-thumbnail?v=d" })} />,
    );
    expect(fetchSeekThumbnailSprite).toHaveBeenCalledTimes(1);
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(1);
  });
});
