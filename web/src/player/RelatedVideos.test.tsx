import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import type { RelatedState } from "../api/useVideoDetail";
import RelatedVideos from "./RelatedVideos";

function item(id: number, overrides: Partial<Video> = {}): Video {
  return {
    id,
    title: `関連 ${String(id)}`,
    public: false,
    sizeBytes: 1,
    addedAt: "2026-09-01T00:00:00Z",
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

function renderList(state: RelatedState) {
  return render(
    <MemoryRouter>
      <RelatedVideos state={state} backTo="/folders/1/a" onRetry={vi.fn()} />
    </MemoryRouter>,
  );
}

describe("RelatedVideos", () => {
  it("各項目はサムネイル・長さ・題名だけで、追加日時などの文字を出さない", () => {
    renderList({ kind: "ready", id: 1, related: { items: [item(2)], nextId: 2 } });
    const link = screen.getByRole("link", { name: /関連 2/ });
    expect(link.getAttribute("href")).toBe("/videos/2");
    expect(link.textContent).toBe("1:05関連 2");
    expect(screen.getByRole("heading", { level: 2, name: "関連動画" })).toBeDefined();
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
    expect(screen.getByText("関連動画を取得できませんでした")).toBeDefined();
    expect(screen.getByRole("button", { name: "再試行" })).toBeDefined();
  });

  it("サムネイルが無ければ一覧のカードと同じ代わりの表示にする", () => {
    renderList({
      kind: "ready",
      id: 1,
      related: {
        items: [item(2, { thumbnailUrl: undefined, thumbnailState: "pending" })],
      },
    });
    expect(screen.getByText("準備中")).toBeDefined();
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
    // リンクの名前は題名と長さだけで、割合は進捗バーとして別に読める。
    expect(screen.getByRole("link", { name: "関連 2 1:05" })).toBeDefined();
    expect(screen.getByRole("progressbar", { name: "再生済みの割合" })).toBeDefined();
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
    const folder = { rootId: 1, path: "series" };
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
          group: { folder, name: "series", items: members },
        },
      });
    }

    it("「続けて再生」と何本目かの下に全メンバーを順に出し、境目の下に関連動画を別の並びで出す（受け入れ条件 15）", () => {
      renderGroup(13);
      const heading = screen.getByRole("heading", { level: 2, name: "続けて再生" });
      expect(heading.parentElement?.textContent).toBe("続けて再生3 / 4");
      const lists = screen.getAllByRole("list");
      expect(lists).toHaveLength(2);
      const [memberList, relatedList] = lists as [HTMLElement, HTMLElement];
      const rows = within(memberList).getAllByRole("listitem");
      expect(rows.map((row) => row.textContent)).toEqual([
        "11:05ep01",
        "21:05ep02",
        "31:05再生中ep03",
        "41:05ep04",
      ]);
      // 境目は「関連動画」の見出しの前にある。
      const separator = screen.getByRole("separator");
      const related = screen.getByRole("heading", { level: 2, name: "関連動画" });
      expect(
        separator.compareDocumentPosition(related) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      expect(
        within(relatedList).getByRole("link", { name: "関連 2 1:05" }),
      ).toBeDefined();
    });

    it("今のメンバーはリンクにせず aria-current で示し、面と左の線で強調する", () => {
      renderGroup(13);
      const current = screen
        .getAllByRole("listitem")
        .find((row) => row.getAttribute("aria-current") === "true");
      expect(current?.textContent).toContain("ep03");
      expect(current && within(current).queryByRole("link")).toBeNull();
      const surface = current?.firstElementChild;
      expect(surface?.className).toContain("bg-active-wash");
      expect(surface?.className).toContain("border-l-2");
      expect(surface?.className).toContain("border-accent");
      expect(screen.queryByRole("link", { name: /ep03/ })).toBeNull();
    });

    it("視聴済みは読み上げ名に「視聴済み」を足し、途中のメンバーは進捗バーを出す", () => {
      renderGroup(13);
      const done = screen.getByRole("link", { name: "ep01 1:05 視聴済み" });
      expect(done.getAttribute("href")).toBe("/videos/11");
      expect(within(done).queryByRole("progressbar")).toBeNull();
      const partial = screen.getByRole("link", { name: "ep02 1:05" });
      expect(within(partial).getByRole("progressbar")).toBeDefined();
      expect(screen.getByRole("link", { name: "ep04 1:05" })).toBeDefined();
    });

    it("関連動画が 0 件なら、境目と「関連動画」の見出しを出さない", () => {
      renderGroup(11, []);
      expect(screen.getByRole("heading", { name: "続けて再生" })).toBeDefined();
      expect(screen.queryByRole("heading", { name: "関連動画" })).toBeNull();
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
        expect(options).toEqual({ block: "nearest" });
        scrolled.push(this);
      };
      try {
        renderList({
          kind: "ready",
          id: 299,
          related: { items: [], group: { folder, name: "big", items: many } },
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
