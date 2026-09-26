import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FolderPreview } from "../api/client";
import FolderArt from "./FolderArt";

const previews: FolderPreview[] = [
  {
    videoId: 1,
    thumbnailUrl: "/api/videos/1/thumbnail?v=x",
    previewUrl: "/api/videos/1/preview?v=x",
  },
  {
    videoId: 2,
    thumbnailUrl: "/api/videos/2/thumbnail?v=x",
    previewUrl: "/api/videos/2/preview?v=x",
  },
];

type ArtProps = Parameters<typeof FolderArt>[0];

function renderArt(props: Partial<ArtProps> = {}) {
  const view = render(<FolderArt previews={previews} {...props} />);
  const art = view.container.querySelector<HTMLElement>("[data-folder-art]");
  if (art === null) throw new Error("folder art not found");
  art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
  const rerender = (next: Partial<ArtProps>) =>
    view.rerender(<FolderArt previews={previews} {...props} {...next} />);
  return {
    art,
    rerender,
    front: () => art.querySelector("[data-folder-front]"),
    videos: () => art.querySelectorAll("video").length,
  };
}

describe("FolderArt の一覧のプレビューの調整", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("選択中は前に出さず、選択に入ると流している1枚を戻す", () => {
    const { art, rerender, front, videos } = renderArt({ selectionMode: true });
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
    act(() => vi.advanceTimersByTime(500));
    expect(front()).toBeNull();

    rerender({ selectionMode: false });
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
    act(() => vi.advanceTimersByTime(500));
    expect(videos()).toBe(1);

    rerender({ selectionMode: true });
    expect(front()).toBeNull();
    expect(videos()).toBe(0);
  });

  it("流し始めた1枚を知らせ、ほかのカードが流し始めるか止める指示で戻す", () => {
    const onPreviewStart = vi.fn();
    const { art, rerender, front, videos } = renderArt({
      activePreviewId: null,
      previewResetEpoch: 0,
      onPreviewStart,
    });

    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 150 });
    act(() => vi.advanceTimersByTime(500));
    expect(onPreviewStart).toHaveBeenCalledWith(2);
    rerender({ activePreviewId: 2, previewResetEpoch: 0 });
    expect(videos()).toBe(1);

    // ほかのカード（動画 9）が流し始めた。
    rerender({ activePreviewId: 9, previewResetEpoch: 0 });
    expect(front()).toBeNull();
    expect(videos()).toBe(0);

    // もう一度流し、表示倍率の変更などで止める指示が来た。
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
    act(() => vi.advanceTimersByTime(500));
    expect(onPreviewStart).toHaveBeenLastCalledWith(1);
    rerender({ activePreviewId: 1, previewResetEpoch: 0 });
    expect(videos()).toBe(1);
    rerender({ activePreviewId: 1, previewResetEpoch: 1 });
    expect(front()).toBeNull();
    expect(videos()).toBe(0);
  });
});
