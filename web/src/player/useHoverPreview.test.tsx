import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { type HoverPreview, useHoverPreview } from "./useHoverPreview";

function video(): Video {
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
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "done",
    previewUrl: "/api/videos/1/preview?v=key",
    tags: [],
  };
}

let current: HoverPreview | undefined;

function Harness() {
  const preview = useHoverPreview(video());
  current = preview;
  return (
    <div
      data-testid="thumbnail"
      onPointerEnter={preview.onPointerEnter}
      onPointerLeave={preview.onPointerLeave}
    >
      {preview.active && (
        <video
          ref={preview.videoRef}
          src={preview.previewUrl}
          muted
          onPlaying={preview.onPlaying}
        />
      )}
    </div>
  );
}

function preview(): HoverPreview {
  if (current === undefined) throw new Error("preview がありません");
  return current;
}

function previewVideo(): HTMLVideoElement | null {
  return document.querySelector("video");
}

function enter() {
  const thumbnail = document.querySelector('[data-testid="thumbnail"]');
  if (thumbnail === null) throw new Error("サムネイルがありません");
  fireEvent.pointerEnter(thumbnail, { pointerType: "mouse" });
}

describe("useHoverPreview suspend and resume", () => {
  const play = vi.fn(() => Promise.resolve());
  const pause = vi.fn();
  const load = vi.fn();

  beforeEach(() => {
    current = undefined;
    vi.useFakeTimers();
    play.mockReset();
    play.mockImplementation(() => Promise.resolve());
    pause.mockReset();
    load.mockReset();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(load);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("待ちの間に止めるとtimerが消え、再開で400msから数え直す", () => {
    render(<Harness />);
    enter();
    act(() => vi.advanceTimersByTime(300));
    act(() => preview().suspendPreview());
    act(() => vi.advanceTimersByTime(1000));
    expect(previewVideo()).toBeNull();

    act(() => preview().resumePreview());
    act(() => vi.advanceTimersByTime(399));
    expect(previewVideo()).toBeNull();
    act(() => vi.advanceTimersByTime(1));
    expect(previewVideo()).not.toBeNull();
    expect(play).toHaveBeenCalledTimes(1);
  });

  it("再生中に止めるとpause()し、再開でplay()する", () => {
    render(<Harness />);
    enter();
    act(() => vi.advanceTimersByTime(400));
    const element = previewVideo();
    if (element === null) throw new Error("プレビューがありません");
    fireEvent.playing(element);
    expect(preview().playing).toBe(true);

    act(() => preview().suspendPreview());
    expect(pause).toHaveBeenCalledTimes(1);
    expect(previewVideo()).toBe(element);
    expect(element.getAttribute("src")).toBe("/api/videos/1/preview?v=key");
    expect(preview().playing).toBe(true);

    act(() => preview().resumePreview());
    expect(play).toHaveBeenCalledTimes(2);
    expect(previewVideo()).toBe(element);
    expect(load).not.toHaveBeenCalled();
  });

  it("読み込み中に止めるとsrcと要素を保ち、再開まで再生を始めない", async () => {
    let abortPlay: ((error: Error) => void) | undefined;
    play.mockImplementationOnce(
      () =>
        new Promise<void>((_resolve, reject) => {
          abortPlay = reject;
        }),
    );
    render(<Harness />);
    enter();
    act(() => vi.advanceTimersByTime(400));
    const element = previewVideo();
    if (element === null) throw new Error("プレビューがありません");
    expect(preview().playing).toBe(false);

    act(() => preview().suspendPreview());
    expect(pause).toHaveBeenCalledTimes(1);
    await act(async () => {
      abortPlay?.(new DOMException("aborted", "AbortError"));
      await Promise.resolve();
    });
    act(() => vi.advanceTimersByTime(5000));
    expect(previewVideo()).toBe(element);
    expect(element.getAttribute("src")).toBe("/api/videos/1/preview?v=key");
    expect(load).not.toHaveBeenCalled();
    expect(play).toHaveBeenCalledTimes(1);

    act(() => preview().resumePreview());
    expect(play).toHaveBeenCalledTimes(2);
    expect(previewVideo()).toBe(element);
  });

  it("止めている間にサムネイルの外へ出たら、今までどおり解放する", () => {
    render(<Harness />);
    enter();
    act(() => vi.advanceTimersByTime(400));
    act(() => preview().suspendPreview());
    const thumbnail = document.querySelector('[data-testid="thumbnail"]');
    if (thumbnail === null) throw new Error("サムネイルがありません");
    fireEvent.pointerLeave(thumbnail, { pointerType: "mouse" });
    expect(previewVideo()).toBeNull();

    act(() => preview().resumePreview());
    act(() => vi.advanceTimersByTime(1000));
    expect(previewVideo()).toBeNull();
  });
});
