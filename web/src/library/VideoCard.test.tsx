import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import VideoCard from "./VideoCard";

function video(extra: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "動画 1",
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    playable: false,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: "/thumbnail.jpg",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "done",
    previewUrl: "/api/videos/1/preview?v=key",
    ...extra,
  };
}

function renderCard(
  item: Video = video(),
  props: Partial<React.ComponentProps<typeof VideoCard>> = {},
) {
  return render(
    <MemoryRouter>
      <VideoCard
        video={item}
        backTo="/"
        selected={false}
        selectionMode={false}
        onSelect={vi.fn()}
        {...props}
      />
    </MemoryRouter>,
  );
}

describe("VideoCard hover preview", () => {
  const play = vi.fn(() => Promise.resolve());
  const pause = vi.fn();
  const load = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(load);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("waits exactly 400ms and keeps the thumbnail until playing", async () => {
    renderCard(video({ unplayableReason: "container" }));
    const card = screen.getByRole("article");
    expect(screen.queryByText("再生に必要な情報がありません")).toBeNull();

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(399));
    expect(document.querySelector("video")).toBeNull();

    act(() => vi.advanceTimersByTime(1));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    expect(preview.getAttribute("src")).toBe("/api/videos/1/preview?v=key");
    expect(play).toHaveBeenCalledTimes(1);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(false);

    fireEvent.playing(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);
    expect(preview.muted).toBe(true);
    expect(preview.playsInline).toBe(true);
    expect(preview.controls).toBe(false);
    await act(async () => Promise.resolve());
  });

  it("cancels short, touch, pen, focus, checkbox, and ineligible hovers", () => {
    const onStart = vi.fn();
    const { rerender } = renderCard(video(), { onPreviewStart: onStart });
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    fireEvent.pointerLeave(card);
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    for (const pointerType of ["touch", "pen"]) {
      fireEvent.pointerEnter(card, { pointerType });
      act(() => vi.advanceTimersByTime(400));
    }
    expect(onStart).not.toHaveBeenCalled();

    fireEvent.focus(screen.getByRole("link"));
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    fireEvent.pointerEnter(screen.getByRole("checkbox").parentElement!, {
      pointerType: "mouse",
    });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    rerender(
      <MemoryRouter>
        <VideoCard
          video={video({ previewState: "pending", previewUrl: undefined })}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          onPreviewStart={onStart}
        />
      </MemoryRouter>,
    );
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("returns to the thumbnail on leave, media error, or play rejection and retries", async () => {
    const onStart = vi.fn();
    renderCard(video(), { onPreviewStart: onStart });
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    fireEvent.error(preview);
    expect(document.querySelector("video")).toBeNull();
    expect(load).toHaveBeenCalled();

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).toHaveBeenCalledTimes(2);

    play.mockImplementationOnce(() => Promise.reject(new Error("blocked")));
    fireEvent.pointerLeave(card);
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    await act(async () => Promise.resolve());
    expect(document.querySelector("video")).toBeNull();
    expect(pause).toHaveBeenCalled();
  });

  it("does not let an old play rejection cancel a newer hover lifecycle", async () => {
    let rejectOldPlay: ((reason?: unknown) => void) | undefined;
    play.mockImplementationOnce(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectOldPlay = reject;
        }),
    );
    renderCard();
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    fireEvent.pointerLeave(card);
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    await act(async () => rejectOldPlay?.(new Error("late rejection")));
    expect(document.querySelector("video")).not.toBeNull();
  });

  it("keeps the current frame while buffering and releases on coordinator reset", () => {
    const { rerender } = renderCard(video(), { activePreviewId: 1 });
    const card = screen.getByRole("article");
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;

    fireEvent.playing(preview);
    fireEvent.waiting(preview);
    fireEvent.stalled(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);
    expect(document.querySelector("video")).not.toBeNull();

    rerender(
      <MemoryRouter>
        <VideoCard
          video={video()}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          activePreviewId={1}
          previewResetEpoch={1}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("removes an inactive coordinated preview in the active-card render", async () => {
    const item = video();
    const onStart = vi.fn();
    const rendered = renderCard(item, {
      activePreviewId: item.id,
      onPreviewStart: onStart,
    });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    fireEvent.playing(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);

    rendered.rerender(
      <MemoryRouter>
        <VideoCard
          video={item}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          activePreviewId={2}
          onPreviewStart={onStart}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(false);
    await act(async () => Promise.resolve());
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("does not suppress an existing warning when a preview URL is present", () => {
    const onStart = vi.fn();
    renderCard(video({ durationMs: undefined }), {
      activePreviewId: 1,
      onPreviewStart: onStart,
    });
    expect(screen.getByText("再生に必要な情報がありません")).toBeDefined();

    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(screen.getByText("再生に必要な情報がありません")).toBeDefined();
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("releases the media element before unmount detaches its ref", () => {
    const rendered = renderCard(video(), { activePreviewId: 1 });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    rendered.unmount();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("stops preview when selection mode starts and keeps card selection priority", () => {
    const onSelect = vi.fn();
    const onReset = vi.fn();
    const item = video();
    const rendered = renderCard(item, {
      activePreviewId: item.id,
      onSelect,
      onPreviewReset: onReset,
    });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    rendered.rerender(
      <MemoryRouter>
        <VideoCard
          video={item}
          backTo="/"
          selected={false}
          selectionMode
          onSelect={onSelect}
          activePreviewId={item.id}
          onPreviewReset={onReset}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: item.title }));
    expect(onReset).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(item.id, true);
  });
});
