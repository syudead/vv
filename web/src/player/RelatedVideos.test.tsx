import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  listVideoGroupMembers,
  type SeekThumbnailSprite,
  type Video,
} from "../api/client";
import type { RelatedState } from "../api/useVideoDetail";
import RelatedVideos from "./RelatedVideos";

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  fetchSeekThumbnailSprite: vi.fn(),
  fetchSeekThumbnailSheet: vi.fn(),
  listVideoGroupMembers: vi.fn(),
}));

function item(id: number, overrides: Partial<Video> = {}): Video {
  return {
    id,
    virtual: false,
    title: `関連 ${String(id)}`,
    public: false,
    sizeBytes: 1,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    previewState: "done",
    durationMs: 65_000,
    tags: [],
    ...overrides,
  };
}

const linkState = { from: "/folders/1/a", trail: [5, 1] };

function renderList(state: RelatedState, onLoadMore = vi.fn()) {
  return render(
    <MemoryRouter>
      <RelatedVideos
        state={state}
        linkState={linkState}
        onRetry={vi.fn()}
        onLoadMore={onLoadMore}
      />
    </MemoryRouter>,
  );
}

describe("RelatedVideos", () => {
  it("各項目はサムネイル・長さ・題名だけで、追加日時などの文字を出さない", () => {
    renderList({ kind: "ready", id: 1, related: { items: [item(2)], nextId: 2 } });
    const link = screen.getByRole("link", { name: /関連 2/ });
    expect(link.getAttribute("href")).toBe("/videos/2");
    expect(link.textContent).toBe("1:05関連 2");
    expect(
      screen.getByRole("heading", { level: 2, name: "Related videos" }),
    ).toBeDefined();
  });

  it("途中まで見た動画だけに進捗バーを出す", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [
          item(2, {
            progress: {
              positionMs: 13_000,
              completed: false,
              updatedAt: "2026-09-02T00:00:00Z",
            },
          }),
          item(3, {
            progress: {
              positionMs: 65_000,
              completed: true,
              updatedAt: "2026-09-02T00:00:00Z",
            },
          }),
          item(4),
        ],
      },
    });
    const [first, second, third] = screen.getAllByRole("link");
    if (first === undefined || second === undefined || third === undefined) {
      throw new Error("項目が足りません");
    }
    expect(within(first).getByRole("progressbar").getAttribute("aria-valuenow")).toBe(
      "20",
    );
    expect(within(second).queryByRole("progressbar")).toBeNull();
    expect(within(third).queryByRole("progressbar")).toBeNull();
  });

  it("0 件のときは見出しも並びも出さない", () => {
    renderList({ kind: "ready", id: 1, related: { items: [] } });
    expect(screen.queryByRole("heading")).toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("読み込み失敗では文言と再試行を出す", () => {
    renderList({ kind: "failed", id: 1 });
    expect(screen.getByText("Couldn't load related videos")).toBeDefined();
    expect(screen.getByRole("button", { name: "Retry" })).toBeDefined();
  });

  it("サムネイルが無ければ一覧のカードと同じ代わりの表示にする", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [item(2, { thumbnailUrl: undefined, thumbnailState: "pending" })],
      },
    });
    expect(screen.getByText("Preparing")).toBeDefined();
  });

  it("リンクの読み上げ名は長さと題名だけで、進捗の数値を含めない", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [
          item(2, {
            progress: {
              positionMs: 32_500,
              completed: false,
              updatedAt: "2026-09-02T00:00:00Z",
            },
          }),
        ],
      },
    });
    expect(screen.getByRole("link", { name: "関連 2 1:05" })).toBeDefined();
    expect(screen.getByRole("progressbar", { name: "Watched portion" })).toBeDefined();
  });

  describe("マウスを乗せたときのプレビュー", () => {
    afterEach(() => {
      vi.useRealTimers();
    });

    const previewItem = (overrides: Partial<Video> = {}) =>
      item(2, { previewUrl: "/api/videos/2/preview?v=a", ...overrides });
    const previewVideo = (container: HTMLElement) =>
      container.querySelector<HTMLVideoElement>("video");

    it("マウスを乗せて 400ms 後に一覧用プレビューを無音で流し、離すと止める", () => {
      vi.useFakeTimers();
      const { container } = renderList({
        kind: "ready",
        id: 1,
        related: { items: [previewItem()], nextId: 2 },
      });
      const link = screen.getByRole("link", { name: "関連 2 1:05" });

      fireEvent.pointerEnter(link, { pointerType: "mouse" });
      act(() => {
        vi.advanceTimersByTime(399);
      });
      expect(previewVideo(container)).toBeNull();
      act(() => {
        vi.advanceTimersByTime(1);
      });
      const video = previewVideo(container);
      expect(video?.getAttribute("src")).toBe("/api/videos/2/preview?v=a");
      expect(video?.muted).toBe(true);
      expect(video?.loop).toBe(true);

      fireEvent.pointerLeave(link, { pointerType: "mouse" });
      expect(previewVideo(container)).toBeNull();
    });

    it("タッチ・ペンや、プレビューが未完成の動画では流さない", () => {
      vi.useFakeTimers();
      const { container, unmount } = renderList({
        kind: "ready",
        id: 1,
        related: { items: [previewItem()], nextId: 2 },
      });
      const link = screen.getByRole("link", { name: "関連 2 1:05" });
      for (const pointerType of ["touch", "pen"]) {
        fireEvent.pointerEnter(link, { pointerType });
        act(() => {
          vi.advanceTimersByTime(1000);
        });
        expect(previewVideo(container)).toBeNull();
      }
      unmount();

      const pending = renderList({
        kind: "ready",
        id: 1,
        related: { items: [previewItem({ previewState: "pending" })], nextId: 2 },
      });
      fireEvent.pointerEnter(screen.getByRole("link", { name: "関連 2 1:05" }), {
        pointerType: "mouse",
      });
      act(() => {
        vi.advanceTimersByTime(1000);
      });
      expect(previewVideo(pending.container)).toBeNull();
    });
  });

  describe("グループのメンバーの並び", () => {
    const watched = {
      positionMs: 65_000,
      completed: true,
      updatedAt: "2026-09-02T00:00:00Z",
    };
    const halfway = {
      positionMs: 13_000,
      completed: false,
      updatedAt: "2026-09-02T00:00:00Z",
    };
    const members = [
      item(11, { title: "ep01", progress: watched }),
      item(12, { title: "ep02", progress: halfway }),
      item(13, { title: "ep03" }),
      item(14, { title: "ep04" }),
    ];

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    function renderGroup(current: number, items: Video[] = [item(2)]) {
      return renderList({
        kind: "ready",
        id: current,
        related: {
          items,
          group: {
            id: 1,
            name: "series",
            items: members,
            offset: 0,
            total: members.length,
          },
        },
      });
    }

    it("「続けて再生」と何本目かの下に全メンバーを順に出し、境目の下に関連動画を別の並びで出す（受け入れ条件 15）", () => {
      renderGroup(13);
      const heading = screen.getByRole("heading", { level: 2, name: "Up next" });
      expect(heading.parentElement?.textContent).toBe("Up next3 / 4");
      const lists = screen.getAllByRole("list");
      expect(lists).toHaveLength(2);
      const [memberList, relatedList] = lists as [HTMLElement, HTMLElement];
      const rows = within(memberList).getAllByRole("listitem");
      expect(rows.map((row) => row.textContent)).toEqual([
        "1:051ep01",
        "1:052ep02",
        "1:053Now playingep03",
        "1:054ep04",
      ]);
      const separator = screen.getByRole("separator");
      const related = screen.getByRole("heading", { level: 2, name: "Related videos" });
      expect(
        separator.compareDocumentPosition(related) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      expect(
        within(relatedList).getByRole("link", { name: "関連 2 1:05" }),
      ).toBeDefined();
    });

    it("今のメンバーはリンクにせず aria-current で示し、面と枠で強調する", () => {
      renderGroup(13);
      const current = screen
        .getAllByRole("listitem")
        .find((row) => row.getAttribute("aria-current") === "true");
      expect(current?.textContent).toContain("ep03");
      expect(current && within(current).queryByRole("link")).toBeNull();
      const surface = current?.firstElementChild;
      expect(surface?.className).toContain("bg-accent");
      expect(surface?.className).toContain("ring-2");
      expect(surface?.className).toContain("ring-primary");
      expect(screen.queryByRole("link", { name: /ep03/ })).toBeNull();
    });

    it("視聴済みは読み上げ名に「視聴済み」を足し、途中のメンバーは進捗バーを出す", () => {
      renderGroup(13);
      const done = screen.getByRole("link", { name: "ep01 1:05, watched" });
      expect(done.getAttribute("href")).toBe("/videos/11");
      expect(within(done).queryByRole("progressbar")).toBeNull();
      const partial = screen.getByRole("link", { name: "ep02 1:05" });
      expect(within(partial).getByRole("progressbar")).toBeDefined();
      expect(screen.getByRole("link", { name: "ep04 1:05" })).toBeDefined();
    });

    it("関連動画が 0 件なら、境目と「関連動画」の見出しを出さない", () => {
      renderGroup(11, []);
      expect(screen.getByRole("heading", { name: "Up next" })).toBeDefined();
      expect(screen.queryByRole("heading", { name: "Related videos" })).toBeNull();
      expect(screen.queryByRole("separator")).toBeNull();
      expect(screen.getAllByRole("list")).toHaveLength(1);
    });

    it("数百本でも切らずに出し、広い画面では今のメンバーの行を入れ物の中で見える位置へ動かす", () => {
      const many = Array.from({ length: 300 }, (_, index) =>
        item(100 + index, { title: `big ${String(index + 1)}` }),
      );
      vi.stubGlobal("matchMedia", (query: string) => ({
        matches: query === "(min-width: 64rem)",
      }));
      const scrolled: Element[] = [];
      const original = Element.prototype.scrollIntoView;
      Element.prototype.scrollIntoView = function (this: Element, options) {
        expect(options).toEqual({ block: "center" });
        scrolled.push(this);
      };
      try {
        renderList({
          kind: "ready",
          id: 299,
          related: {
            items: [],
            group: { id: 1, name: "big", items: many, offset: 0, total: many.length },
          },
        });
        expect(screen.getAllByRole("listitem")).toHaveLength(300);
        expect(screen.getByText("200 / 300")).toBeDefined();
        expect(scrolled).toHaveLength(1);
        expect(scrolled[0]?.getAttribute("aria-current")).toBe("true");
        expect(scrolled[0]?.textContent).toContain("big 200");
      } finally {
        Element.prototype.scrollIntoView = original;
      }
    });

    it("狭い画面ではページを動かさない", () => {
      vi.stubGlobal("matchMedia", () => ({ matches: false }));
      const scrollIntoView = vi.fn();
      const original = Element.prototype.scrollIntoView;
      Element.prototype.scrollIntoView = scrollIntoView;
      try {
        renderGroup(14);
        expect(scrollIntoView).not.toHaveBeenCalled();
      } finally {
        Element.prototype.scrollIntoView = original;
      }
    });
  });
});

describe("関連動画のスクラブの帯", () => {
  const play = vi.fn(() => Promise.resolve());
  const pause = vi.fn();
  const sprite: SeekThumbnailSprite = {
    intervalMs: 1_000,
    frameCount: 65,
    columns: 10,
    rows: 10,
    frameWidth: 160,
    frameHeight: 90,
    sheets: ["/api/videos/2/seek-thumbnail/0?v=c"],
  };
  let resolveSprite: ((value: SeekThumbnailSprite) => void) | undefined;
  let resolveSheet: ((value: Blob) => void) | undefined;

  function scrubItem(id: number, overrides: Partial<Video> = {}): Video {
    return item(id, {
      seekThumbnailUrl: `/api/videos/${String(id)}/seek-thumbnail?v=c`,
      previewUrl: `/api/videos/${String(id)}/preview?v=a`,
      ...overrides,
    });
  }

  const bands = () =>
    Array.from(document.querySelectorAll<HTMLElement>("[data-scrub-band]"));
  const scrubBar = () => document.querySelector<HTMLElement>("[data-scrub-bar]");

  function bandOf(link: HTMLElement): HTMLElement {
    const band = link.querySelector<HTMLElement>("[data-scrub-band]");
    if (band === null) throw new Error("no band");
    return band;
  }

  // React derives pointerenter/pointerleave from pointerout between elements.
  // The scrub band starts at 100px and spans 400px; x=300 selects 0:32 of 65 seconds.
  function enterBand(link: HTMLElement, clientX: number): HTMLElement {
    const band = bandOf(link);
    fireEvent.pointerEnter(link, { pointerType: "mouse" });
    fireEvent.pointerOut(link, { pointerType: "mouse", clientX, relatedTarget: band });
    return band;
  }

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(vi.fn());
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
    vi.mocked(fetchSeekThumbnailSprite).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSprite = resolve;
        }),
    );
    vi.mocked(fetchSeekThumbnailSheet).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSheet = resolve;
        }),
    );
    Object.defineProperty(URL, "createObjectURL", {
      value: vi.fn(() => "blob:sheet"),
      configurable: true,
    });
    Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.mocked(fetchSeekThumbnailSprite).mockReset();
    vi.mocked(fetchSeekThumbnailSheet).mockReset();
    play.mockClear();
    pause.mockClear();
    vi.useRealTimers();
  });

  it("帯にいる間だけ長さの表示とバーが差し替わり、帯を出ると戻る", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [
          scrubItem(2, {
            progress: { positionMs: 13_000, completed: false, updatedAt: "" },
          }),
        ],
      },
    });
    const link = screen.getByRole("link", { name: "関連 2 1:05" });
    const progress = within(link).getByRole("progressbar");
    expect(within(link).getByText("1:05")).toBeDefined();
    expect(scrubBar()).toBeNull();

    const band = enterBand(link, 300);
    expect(within(link).getByText("0:32 / 1:05")).toBeDefined();
    expect(scrubBar()?.getAttribute("aria-hidden")).toBe("true");
    expect(scrubBar()?.firstElementChild?.getAttribute("style")).toContain("width: 50%");
    expect(progress.classList.contains("opacity-0")).toBe(true);
    expect(progress.getAttribute("aria-valuenow")).toBe("20");
    expect(link.getAttribute("aria-label")).toBe("関連 2 1:05");
    expect(band.getAttribute("aria-hidden")).toBe("true");

    fireEvent.pointerMove(band, { pointerType: "mouse", clientX: 500 });
    expect(within(link).getByText("1:04 / 1:05")).toBeDefined();
    expect(scrubBar()?.firstElementChild?.getAttribute("style")).toContain("width: 100%");

    fireEvent.pointerLeave(band, { pointerType: "mouse", relatedTarget: link });
    expect(within(link).getByText("1:05")).toBeDefined();
    expect(scrubBar()).toBeNull();
    expect(progress.classList.contains("opacity-0")).toBe(false);
  });

  it("未視聴の動画でも帯にいる間はバーを出し、コマは揃ってから出す", async () => {
    renderList({ kind: "ready", id: 1, related: { items: [scrubItem(2)] } });
    const link = screen.getByRole("link", { name: "関連 2 1:05" });
    expect(within(link).queryByRole("progressbar")).toBeNull();
    enterBand(link, 300);
    expect(scrubBar()).not.toBeNull();
    expect(document.querySelector("[data-scrub-frame]")).toBeNull();

    await act(async () => resolveSprite?.(sprite));
    await act(async () => resolveSheet?.(new Blob()));
    const frame = document.querySelector("[data-scrub-frame]");
    expect(frame?.getAttribute("aria-hidden")).toBe("true");
    expect(link.contains(frame)).toBe(true);
  });

  it("タッチ・ペンでは帯が反応しない", () => {
    renderList({ kind: "ready", id: 1, related: { items: [scrubItem(2)] } });
    const link = screen.getByRole("link", { name: "関連 2 1:05" });
    const band = bandOf(link);
    for (const pointerType of ["touch", "pen"]) {
      fireEvent.pointerOut(link, { pointerType, clientX: 300, relatedTarget: band });
      fireEvent.pointerMove(band, { pointerType, clientX: 300 });
      expect(scrubBar()).toBeNull();
    }
    expect(fetchSeekThumbnailSprite).not.toHaveBeenCalled();
  });

  it("ループ再生中に帯へ入ると止め、帯から上へ出ると再開し、リンクの外へ出ると解放する", () => {
    renderList({ kind: "ready", id: 1, related: { items: [scrubItem(2)] } });
    const link = screen.getByRole("link", { name: "関連 2 1:05" });
    fireEvent.pointerEnter(link, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(play).toHaveBeenCalledTimes(1);

    const band = bandOf(link);
    fireEvent.pointerOut(link, {
      pointerType: "mouse",
      clientX: 200,
      relatedTarget: band,
    });
    expect(pause).toHaveBeenCalledTimes(1);
    fireEvent.pointerLeave(band, { pointerType: "mouse", relatedTarget: link });
    expect(play).toHaveBeenCalledTimes(2);

    fireEvent.pointerOut(link, {
      pointerType: "mouse",
      clientX: 200,
      relatedTarget: band,
    });
    fireEvent.pointerLeave(band, { pointerType: "mouse", relatedTarget: document.body });
    fireEvent.pointerLeave(link, { pointerType: "mouse", relatedTarget: document.body });
    expect(play).toHaveBeenCalledTimes(2);
    expect(document.querySelector("video")).toBeNull();
    expect(scrubBar()).toBeNull();
  });

  it("帯にいるまま窓の大きさが変わると、帯から出たのと同じに戻してループを解放する", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [
          scrubItem(2, {
            progress: { positionMs: 13_000, completed: false, updatedAt: "" },
          }),
        ],
      },
    });
    const link = screen.getByRole("link", { name: "関連 2 1:05" });
    const progress = within(link).getByRole("progressbar");
    fireEvent.pointerEnter(link, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();
    const band = bandOf(link);
    fireEvent.pointerOut(link, {
      pointerType: "mouse",
      clientX: 300,
      relatedTarget: band,
    });
    expect(within(link).getByText("0:32 / 1:05")).toBeDefined();

    act(() => {
      window.dispatchEvent(new Event("resize"));
    });
    expect(within(link).getByText("1:05")).toBeDefined();
    expect(scrubBar()).toBeNull();
    expect(progress.classList.contains("opacity-0")).toBe(false);
    expect(document.querySelector("video")).toBeNull();
    act(() => vi.advanceTimersByTime(400));
    expect(play).toHaveBeenCalledTimes(1);
  });

  it("帯の上のクリックはその動画へ移る", () => {
    render(
      <MemoryRouter>
        <Routes>
          <Route
            path="/"
            element={
              <RelatedVideos
                state={{ kind: "ready", id: 1, related: { items: [scrubItem(2)] } }}
                linkState={{ from: "/", trail: [1] }}
                onRetry={vi.fn()}
                onLoadMore={vi.fn()}
              />
            }
          />
          <Route path="/videos/:id" element={<p>player</p>} />
        </Routes>
      </MemoryRouter>,
    );
    const band = enterBand(screen.getByRole("link", { name: "関連 2 1:05" }), 300);
    fireEvent.click(band);
    expect(screen.getByText("player")).toBeDefined();
  });

  it("スプライトや長さの無い動画には帯が無い", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [
          scrubItem(2),
          scrubItem(3, { seekThumbnailUrl: undefined }),
          scrubItem(4, { durationMs: 0 }),
        ],
      },
    });
    const links = screen.getAllByRole("link");
    expect(links.map((link) => link.querySelector("[data-scrub-band]") !== null)).toEqual(
      [true, false, false],
    );
  });

  it("グループのメンバーの行と関連動画には帯を付け、今見ているメンバーの行には付けない", () => {
    const { container } = renderList({
      kind: "ready",
      id: 12,
      related: {
        items: [scrubItem(2)],
        group: {
          id: 1,
          name: "series",
          items: [scrubItem(11), scrubItem(12), scrubItem(13)],
          offset: 0,
          total: 3,
        },
      },
    });
    const current = container.querySelector("[aria-current]");
    expect(current?.querySelector("[data-scrub-band]")).toBeNull();
    expect(bands()).toHaveLength(3);
    for (const band of bands()) expect(band.closest("a")).not.toBeNull();
  });

  describe("大きなグループの窓（issue 674）", () => {
    const member = (position: number) =>
      item(position, { title: `ep ${String(position)}` });
    // Center video 3000 in the 2950-3049 window of 6,000 members.
    const window = Array.from({ length: 100 }, (_, index) => member(2950 + index));
    let intersect: Map<Element, IntersectionObserverCallback>;

    beforeEach(() => {
      intersect = new Map();
      vi.stubGlobal(
        "IntersectionObserver",
        class {
          private readonly callback: IntersectionObserverCallback;
          constructor(callback: IntersectionObserverCallback) {
            this.callback = callback;
          }
          observe(element: Element) {
            intersect.set(element, this.callback);
          }
          disconnect() {}
        },
      );
      vi.mocked(listVideoGroupMembers).mockReset();
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    function wide(matches: boolean) {
      vi.stubGlobal("matchMedia", (query: string) => ({
        matches: matches && query === "(min-width: 64rem)",
      }));
    }

    function renderWindow() {
      return renderList({
        kind: "ready",
        id: 3000,
        related: {
          items: [],
          group: { id: 1, name: "big", items: window, offset: 2949, total: 6000 },
        },
      });
    }

    function memberRows() {
      return within(screen.getAllByRole("list")[0]!).getAllByRole("listitem");
    }

    // Select pagination sentinels only; thumbnails also use IntersectionObserver.
    function edges() {
      return [...intersect.entries()].filter(
        ([element]) => element.isConnected && element.classList.contains("h-px"),
      );
    }

    async function reachEdge(index: number) {
      const [element, callback] = edges()[index]!;
      await act(async () => {
        callback(
          [
            {
              isIntersecting: true,
              target: element,
            } as unknown as IntersectionObserverEntry,
          ],
          {} as IntersectionObserver,
        );
        await Promise.resolve();
      });
    }

    it("窓の番号と本数はグループ全体で数え、後ろの端に来たら続きを読み足す", async () => {
      wide(false);
      vi.mocked(listVideoGroupMembers).mockResolvedValue({
        items: [member(3050), member(3051)],
        offset: 3049,
        total: 6000,
      });
      renderWindow();
      expect(screen.getByText("3,000 / 6,000")).toBeDefined();
      expect(memberRows()).toHaveLength(100);
      expect(memberRows()[0]?.textContent).toContain("2950");

      expect(edges()).toHaveLength(1);
      await reachEdge(0);
      expect(listVideoGroupMembers).toHaveBeenCalledWith(
        3000,
        3049,
        100,
        expect.anything(),
        undefined,
        undefined,
      );
      expect(memberRows()).toHaveLength(102);
      expect(memberRows()[101]?.textContent).toContain("ep 3051");
    });

    it("同じ動画のまま応答のグループが替わったら、前のグループで読み足した行を捨てる", async () => {
      wide(false);
      vi.mocked(listVideoGroupMembers).mockResolvedValue({
        items: [member(3050), member(3051)],
        offset: 3049,
        total: 6000,
      });
      const stateOf = (groupId: number): RelatedState => ({
        kind: "ready",
        id: 3000,
        related: {
          items: [],
          group: { id: groupId, name: "big", items: window, offset: 2949, total: 6000 },
        },
      });
      const tree = (state: RelatedState) => (
        <MemoryRouter>
          <RelatedVideos
            state={state}
            linkState={linkState}
            onRetry={vi.fn()}
            onLoadMore={vi.fn()}
          />
        </MemoryRouter>
      );
      const { rerender } = render(tree(stateOf(1)));
      await reachEdge(0);
      expect(memberRows()).toHaveLength(102);

      rerender(tree(stateOf(2)));
      expect(memberRows()).toHaveLength(100);
      expect(screen.queryByText("ep 3051")).toBeNull();
    });

    it("同じグループのまま最初のページを読み直したら（generation）、読み足した行と読み足し中の要求を捨てる", async () => {
      wide(false);
      vi.mocked(listVideoGroupMembers).mockResolvedValueOnce({
        items: [member(3050), member(3051)],
        offset: 3049,
        total: 6000,
      });
      const stateOf = (generation: number): RelatedState => ({
        kind: "ready",
        id: 3000,
        related: {
          items: [],
          group: { id: 1, name: "big", items: window, offset: 2949, total: 6000 },
        },
        generation,
      });
      const tree = (state: RelatedState) => (
        <MemoryRouter>
          <RelatedVideos
            state={state}
            linkState={linkState}
            onRetry={vi.fn()}
            onLoadMore={vi.fn()}
          />
        </MemoryRouter>
      );
      const { rerender } = render(tree(stateOf(1)));
      await reachEdge(0);
      expect(memberRows()).toHaveLength(102);

      rerender(tree(stateOf(2)));
      expect(memberRows()).toHaveLength(100);
      expect(screen.queryByText("ep 3051")).toBeNull();

      let signal: AbortSignal | undefined;
      vi.mocked(listVideoGroupMembers).mockImplementationOnce(
        (_id, _start, _limit, s) => {
          signal = s;
          return new Promise(() => undefined);
        },
      );
      await reachEdge(0);
      expect(signal?.aborted).toBe(false);
      rerender(tree(stateOf(3)));
      expect(signal?.aborted).toBe(true);
    });

    it("狭い画面では、前のメンバーをボタンで読む", async () => {
      wide(false);
      vi.mocked(listVideoGroupMembers).mockResolvedValue({
        items: Array.from({ length: 100 }, (_, index) => member(2850 + index)),
        offset: 2849,
        total: 6000,
      });
      renderWindow();
      fireEvent.click(screen.getByRole("button", { name: "Show 2,949 earlier videos" }));
      await act(async () => Promise.resolve());
      expect(listVideoGroupMembers).toHaveBeenCalledWith(
        3000,
        2849,
        100,
        expect.anything(),
        undefined,
        undefined,
      );
      expect(memberRows()).toHaveLength(200);
      expect(memberRows()[0]?.textContent).toContain("2850");
      expect(screen.getByText("3,000 / 6,000")).toBeDefined();
    });

    it("広い画面では、上の端に来たら前を読み足し、見ていた行が動かないよう位置を補う", async () => {
      wide(true);
      const original = Element.prototype.scrollIntoView;
      Element.prototype.scrollIntoView = vi.fn();
      vi.mocked(listVideoGroupMembers).mockResolvedValue({
        items: [member(2948), member(2949)],
        offset: 2947,
        total: 6000,
      });
      try {
        renderWindow();
        const scroller = document.querySelector("[data-related-scroller]") as HTMLElement;
        Object.defineProperty(scroller, "scrollHeight", {
          get: () => scroller.querySelectorAll("li").length * 10,
        });
        scroller.scrollTop = 40;
        expect(edges()).toHaveLength(2);
        await reachEdge(0);
        expect(listVideoGroupMembers).toHaveBeenCalledWith(
          3000,
          2849,
          100,
          expect.anything(),
          undefined,
          undefined,
        );
        expect(scroller.scrollTop).toBe(60);
        expect(memberRows()[0]?.textContent).toContain("ep 2948");
      } finally {
        Element.prototype.scrollIntoView = original;
      }
    });

    it("幅が lg をまたいだら、前の読み方をボタンとスクロールで切り替える", async () => {
      let matches = false;
      const listeners = new Set<() => void>();
      vi.stubGlobal("matchMedia", (query: string) => ({
        get matches() {
          return matches && query === "(min-width: 64rem)";
        },
        addEventListener: (_: string, listener: () => void) => listeners.add(listener),
        removeEventListener: (_: string, listener: () => void) =>
          listeners.delete(listener),
      }));
      const original = Element.prototype.scrollIntoView;
      Element.prototype.scrollIntoView = vi.fn();
      try {
        renderWindow();
        expect(screen.getByRole("button", { name: /earlier videos/ })).toBeDefined();
        expect(edges()).toHaveLength(1);

        matches = true;
        act(() => {
          for (const listener of listeners) listener();
        });
        expect(screen.queryByRole("button", { name: /earlier videos/ })).toBeNull();
        expect(edges()).toHaveLength(2);

        matches = false;
        act(() => {
          for (const listener of listeners) listener();
        });
        expect(screen.getByRole("button", { name: /earlier videos/ })).toBeDefined();
      } finally {
        Element.prototype.scrollIntoView = original;
      }
    });

    it("読み足しに失敗したら、出ている行を残してやり直しのボタンを出す", async () => {
      wide(false);
      vi.mocked(listVideoGroupMembers).mockRejectedValueOnce(new Error("offline"));
      renderWindow();
      await reachEdge(0);
      expect(memberRows()).toHaveLength(100);
      expect(screen.getByText("Couldn't load more of the group")).toBeDefined();
      vi.mocked(listVideoGroupMembers).mockResolvedValue({
        items: [member(3050)],
        offset: 3049,
        total: 6000,
      });
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      await act(async () => Promise.resolve());
      expect(memberRows()).toHaveLength(101);
    });
  });
});

describe("関連動画の続き（issue 931）", () => {
  let intersect: Map<Element, IntersectionObserverCallback>;

  beforeEach(() => {
    intersect = new Map();
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        private readonly callback: IntersectionObserverCallback;
        constructor(callback: IntersectionObserverCallback) {
          this.callback = callback;
        }
        observe(element: Element) {
          intersect.set(element, this.callback);
        }
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function reachEnd() {
    const edges = [...intersect.entries()].filter(
      ([element]) => element.isConnected && element.classList.contains("h-px"),
    );
    expect(edges).toHaveLength(1);
    const [element, callback] = edges[0]!;
    act(() =>
      callback(
        [
          {
            isIntersecting: true,
            target: element,
          } as unknown as IntersectionObserverEntry,
        ],
        {} as IntersectionObserver,
      ),
    );
  }

  it("続きがあれば、並びの終わりに来たところで続きを読む", () => {
    const onLoadMore = vi.fn();
    renderList(
      { kind: "ready", id: 1, related: { items: [item(2)], nextCursor: "c1" } },
      onLoadMore,
    );
    reachEnd();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("グループのメンバーでも、境目の下の関連動画の終わりで続きを読む", () => {
    const onLoadMore = vi.fn();
    renderList(
      {
        kind: "ready",
        id: 10,
        related: {
          items: [item(2)],
          nextCursor: "c1",
          group: {
            id: 1,
            name: "show",
            items: [item(10), item(11)],
            offset: 0,
            total: 2,
          },
        },
      },
      onLoadMore,
    );
    reachEnd();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("先頭のページが空でも続きがあれば、続きを読む", () => {
    const onLoadMore = vi.fn();
    renderList(
      { kind: "ready", id: 1, related: { items: [], nextCursor: "c1" } },
      onLoadMore,
    );
    reachEnd();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("グループのメンバーで関連動画のページが空でも、続きがあれば続きを読む", () => {
    const onLoadMore = vi.fn();
    renderList(
      {
        kind: "ready",
        id: 10,
        related: {
          items: [],
          nextCursor: "c1",
          group: {
            id: 1,
            name: "show",
            items: [item(10), item(11)],
            offset: 0,
            total: 2,
          },
        },
      },
      onLoadMore,
    );
    reachEnd();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("続きが無ければ目印を置かない", () => {
    renderList({ kind: "ready", id: 1, related: { items: [item(2)] } });
    const edges = [...intersect.keys()].filter(
      (element) => element.isConnected && element.classList.contains("h-px"),
    );
    expect(edges).toHaveLength(0);
  });

  it("続きの読み込みに失敗したら、やり直しのボタンで読み直す", () => {
    const onLoadMore = vi.fn();
    renderList(
      {
        kind: "ready",
        id: 1,
        related: { items: [item(2)], nextCursor: "c1" },
        more: "failed",
      },
      onLoadMore,
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });
});
