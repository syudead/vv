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
});
