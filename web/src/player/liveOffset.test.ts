import { afterEach, describe, expect, it, vi } from "vitest";

import { createLiveOffsetMiddleware, liveSource } from "./liveOffset";

function ranges(values: [number, number][]): TimeRanges {
  return {
    length: values.length,
    start: (index) => values[index]?.[0] ?? 0,
    end: (index) => values[index]?.[1] ?? 0,
  };
}

function fixture() {
  let dispose: (() => void) | undefined;
  let canPlay: (() => void) | undefined;
  const player = {
    one: vi.fn((event: string, callback: () => void) => {
      if (event === "dispose") dispose = callback;
    }),
    paused: vi.fn(() => true),
  };
  const tech = {
    buffered: vi.fn(() => ranges([[0, 10]])),
    setSource: vi.fn(),
    playbackRate: vi.fn(() => 1.5),
    setPlaybackRate: vi.fn(),
    paused: vi.fn(() => false),
    pause: vi.fn(),
    play: vi.fn(),
    scrubbing: vi.fn(() => false),
    one: vi.fn((event: string, callback: () => void) => {
      if (event === "canplay") canPlay = callback;
    }),
  };
  const middleware = createLiveOffsetMiddleware(player as never);
  middleware.setTech(tech);
  return { middleware, tech, dispose: () => dispose?.(), canPlay: () => canPlay?.() };
}

describe("live offset middleware", () => {
  afterEach(() => vi.useRealTimers());

  it("duration、currentTime、bufferedを元動画の時間軸へ写す", () => {
    const { middleware } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000), () => undefined);

    expect(middleware.duration(90)).toBe(120);
    expect(middleware.currentTime(5)).toBe(35);
    const shifted = middleware.buffered(ranges([[0, 10]]));
    expect([shifted.start(0), shifted.end(0)]).toEqual([30, 40]);
  });

  it("buffer済み位置は同じsource内の相対seekにする", () => {
    const { middleware, tech } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000), () => undefined);
    tech.buffered.mockReturnValue(ranges([[0, 20]]));

    expect(middleware.setCurrentTime(42)).toBe(12);
    expect(tech.setSource).not.toHaveBeenCalled();
  });

  it("未buffer位置の連続seekをまとめ、最後の位置からsourceを作り直す", () => {
    vi.useFakeTimers();
    const changed = vi.fn();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 10_000, changed), () => undefined);

    expect(middleware.setCurrentTime(50)).toBe(0);
    expect(middleware.setCurrentTime(70)).toBe(0);
    expect(tech.setSource).not.toHaveBeenCalled();
    vi.advanceTimersByTime(200);

    expect(tech.setSource).toHaveBeenCalledTimes(1);
    expect(tech.setSource.mock.calls[0]?.[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?startMs=70000",
      vvOffsetSeconds: 70,
    });
    expect(changed).toHaveBeenLastCalledWith(70);
    expect(tech.setPlaybackRate).toHaveBeenCalledWith(1.5);
    middleware.callPlay();
    canPlay();
    expect(tech.play).toHaveBeenCalledTimes(1);
  });

  it("disposeで予約済みreloadを破棄する", () => {
    vi.useFakeTimers();
    const { middleware, tech, dispose } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);
    middleware.setCurrentTime(70);
    dispose();
    vi.runAllTimers();
    expect(tech.setSource).not.toHaveBeenCalled();
  });
});
