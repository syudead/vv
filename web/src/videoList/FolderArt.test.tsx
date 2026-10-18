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

    rerender({ activePreviewId: 9, previewResetEpoch: 0 });
    expect(front()).toBeNull();
    expect(videos()).toBe(0);

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

describe("FolderArt の同じ動画を 2 度持つ絵柄", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("同じ動画の 1 枚を重ねて出し、React の key の重複を出さない", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const sheet = (videoId: number): FolderPreview => ({
      videoId,
      thumbnailUrl: `/api/videos/${String(videoId)}/thumbnail?v=x`,
    });
    const view = render(<FolderArt previews={[sheet(1), sheet(2), sheet(1)]} />);
    view.rerender(<FolderArt previews={[sheet(1), sheet(1), sheet(2)]} />);

    expect(view.container.querySelectorAll("[data-folder-preview]")).toHaveLength(3);
    expect(error).not.toHaveBeenCalled();
  });
});

describe("FolderArt の重なり", () => {
  const sheet = (videoId: number): FolderPreview => ({
    videoId,
    thumbnailUrl: `/api/videos/${String(videoId)}/thumbnail?v=x`,
  });
  const zIndexes = (art: Element) =>
    Array.from(art.querySelectorAll<HTMLElement>("[data-folder-preview]"), (element) =>
      Number(element.style.zIndex),
    );

  it("adds a fifth More sheet only when videos remain beyond the four covers", () => {
    const covers = [sheet(1), sheet(2), sheet(3), sheet(4)];
    const view = render(<FolderArt previews={covers} totalCount={4} />);
    expect(view.container.querySelector("[data-folder-more]")).toBeNull();
    view.rerender(<FolderArt previews={covers} totalCount={5} />);
    expect(view.container.querySelectorAll("[data-folder-preview]")).toHaveLength(4);
    expect(view.container.querySelector("[data-folder-more]")?.textContent).toBe(
      "More+1",
    );
  });

  it("指していないときは先頭の1枚ほど手前に重ね、指した1枚はどれよりも手前に出す", () => {
    const view = render(
      <FolderArt previews={[sheet(1), sheet(2), sheet(3), sheet(4)]} />,
    );
    const art = view.container.querySelector<HTMLElement>("[data-folder-art]");
    if (art === null) throw new Error("folder art not found");
    art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);

    const resting = zIndexes(art);
    expect(resting).toEqual([...resting].sort((a, b) => b - a));
    expect(new Set(resting).size).toBe(4);

    fireEvent.pointerMove(art, { clientX: 190, pointerType: "mouse" });
    const pointed = zIndexes(art);
    expect(pointed[3]).toBeGreaterThan(Math.max(...pointed.slice(0, 3)));
  });
});
