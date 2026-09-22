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
  const canPlay: (() => void)[] = [];
  const player = {
    one: vi.fn((event: string, callback: () => void) => {
      if (event === "dispose") dispose = callback;
    }),
    paused: vi.fn(() => true),
  };
  const tech = {
    buffered: vi.fn(() => ranges([[0, 10]])),
    currentTime: vi.fn(() => 4),
    setSource: vi.fn(),
    playbackRate: vi.fn(() => 1.5),
    setPlaybackRate: vi.fn(),
    paused: vi.fn(() => false),
    pause: vi.fn(),
    play: vi.fn(() => Promise.resolve()),
    one: vi.fn((event: string, callback: () => void) => {
      if (event === "canplay") canPlay.push(callback);
    }),
    trigger: vi.fn(),
  };
  const middleware = createLiveOffsetMiddleware(player as never);
  middleware.setTech(tech);
  return {
    middleware,
    tech,
    dispose: () => dispose?.(),
    canPlay: (index = canPlay.length - 1) => canPlay[index]?.(),
  };
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

    expect(middleware.setCurrentTime(50)).toBe(4);
    expect(middleware.setCurrentTime(70)).toBe(4);
    expect(middleware.currentTime(20)).toBe(70);
    expect(tech.setSource).not.toHaveBeenCalled();
    vi.advanceTimersByTime(200);

    expect(tech.setSource).toHaveBeenCalledTimes(1);
    expect(tech.setSource.mock.calls[0]?.[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?startMs=70000",
      vvOffsetSeconds: 70,
    });
    expect(changed).toHaveBeenLastCalledWith(70);
    expect(tech.setPlaybackRate).toHaveBeenCalledWith(1.5);
    expect(tech.trigger).toHaveBeenCalledWith("timeupdate");
    expect(tech.play).toHaveBeenCalledTimes(1);
    middleware.callPlay();
    canPlay();
    expect(tech.play).toHaveBeenCalledTimes(1);
    expect(tech.pause).not.toHaveBeenCalled();
  });

  it("再生中のreloadは新sourceを直ちに読み始めて再生を継続する", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);

    middleware.callPlay();
    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    expect(tech.play).toHaveBeenCalledTimes(1);
    canPlay();

    expect(tech.play).toHaveBeenCalledTimes(1);
    expect(tech.pause).not.toHaveBeenCalled();
  });

  it("一時停止中のreloadは読込後に一時停止へ戻す", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);
    tech.paused.mockReturnValue(true);

    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    expect(tech.play).toHaveBeenCalledTimes(1);
    canPlay();

    expect(tech.pause).toHaveBeenCalledTimes(1);
  });

  it("一時停止中のreloadが完了する前に再seekしても停止意図を維持する", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);
    tech.buffered.mockReturnValue(ranges([]));

    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    expect(tech.play).toHaveBeenCalledTimes(1);
    expect(tech.paused()).toBe(false);

    middleware.setCurrentTime(80);
    vi.advanceTimersByTime(200);
    expect(tech.play).toHaveBeenCalledTimes(2);

    canPlay(0);
    expect(tech.pause).not.toHaveBeenCalled();
    canPlay(1);
    expect(tech.pause).toHaveBeenCalledTimes(1);
  });

  it("reload中に一時停止されたらcanplay後も再生しない", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);

    middleware.callPlay();
    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    middleware.callPause();
    canPlay();

    expect(tech.play).toHaveBeenCalledTimes(1);
    expect(tech.pause).toHaveBeenCalledTimes(1);
  });

  it("古いsourceのcanplayは新しいseekの再生状態を変えない", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);

    middleware.callPlay();
    middleware.setCurrentTime(40);
    vi.advanceTimersByTime(200);
    middleware.setCurrentTime(80);
    vi.advanceTimersByTime(200);
    middleware.callPause();

    canPlay(0);
    expect(tech.pause).not.toHaveBeenCalled();
    canPlay(1);
    expect(tech.pause).toHaveBeenCalledTimes(1);
  });

  it("player側のsource切替は進行中reloadのcanplayを無効化する", () => {
    vi.useFakeTimers();
    const { middleware, tech, canPlay } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0), () => undefined);

    middleware.callPlay();
    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    middleware.callPause();
    middleware.setSource({ src: "/replacement.mp4", type: "video/mp4" }, () => undefined);
    canPlay();

    expect(tech.pause).not.toHaveBeenCalled();
  });

  it("後方seekのreload待機中は選択位置を論理時刻として返す", () => {
    vi.useFakeTimers();
    const { middleware, tech } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000), () => undefined);
    tech.buffered.mockReturnValue(ranges([[0, 10]]));

    expect(middleware.setCurrentTime(5)).toBe(4);
    expect(middleware.currentTime(20)).toBe(5);
  });

  it("未buffer位置の予約後にbuffer内へ戻したら古いreloadを破棄する", () => {
    vi.useFakeTimers();
    const changed = vi.fn();
    const { middleware, tech } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0, changed), () => undefined);

    expect(middleware.setCurrentTime(70)).toBe(4);
    expect(middleware.setCurrentTime(10)).toBe(10);
    vi.runAllTimers();

    expect(tech.setSource).not.toHaveBeenCalled();
    expect(changed).not.toHaveBeenCalled();
    expect(middleware.currentTime(10)).toBe(10);
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
