import { afterEach, describe, expect, it, vi } from "vitest";

import { createLiveOffsetMiddleware, liveSource } from "./liveOffset";
import { createPlayerControls, type ControllablePlayer } from "./playerControls";

function ranges(values: [number, number][]): TimeRanges {
  return {
    length: values.length,
    start: (index) => values[index]?.[0] ?? 0,
    end: (index) => values[index]?.[1] ?? 0,
  };
}

/**
 * transcodePlayer は、video.js が行うのと同じ順で仲立ち（liveOffset）を通す
 * プレイヤーである。位置と速度の読み書きは、仲立ちを経て技術層（tech）へ届く。
 */
function transcodePlayer() {
  let techTime = 4;
  let techRate = 1;
  const tech = {
    buffered: vi.fn(() => ranges([[0, 10]])),
    currentTime: vi.fn(() => techTime),
    setCurrentTime: vi.fn((seconds: number) => {
      techTime = seconds;
    }),
    setSource: vi.fn(),
    playbackRate: vi.fn(() => techRate),
    setPlaybackRate: vi.fn((rate: number) => {
      techRate = rate;
    }),
    paused: vi.fn(() => false),
    pause: vi.fn(),
    play: vi.fn(() => Promise.resolve()),
    one: vi.fn(),
    trigger: vi.fn(),
  };
  const middleware = createLiveOffsetMiddleware({
    one: vi.fn(),
    paused: () => false,
  } as never);
  middleware.setTech(tech);
  middleware.setSource(liveSource(7, 120_000, 30_000), () => undefined);

  const player: ControllablePlayer & { playbackRate(rate?: number): number } = {
    paused: () => false,
    play: () => Promise.resolve(),
    pause: vi.fn(),
    currentTime(seconds?: number) {
      if (seconds === undefined) return middleware.currentTime(tech.currentTime());
      tech.setCurrentTime(middleware.setCurrentTime(seconds));
      return seconds;
    },
    duration: () => middleware.duration(Number.NaN),
    muted: () => false,
    isFullscreen: () => false,
    requestFullscreen: vi.fn(),
    exitFullscreen: vi.fn(),
    userActive: vi.fn(),
    playbackRate(rate?: number) {
      if (rate !== undefined) tech.setPlaybackRate(rate);
      return tech.playbackRate();
    },
  };
  return { player, tech, middleware };
}

describe("createPlayerControls", () => {
  afterEach(() => vi.useRealTimers());

  it("変換して再生する経路でも、→ と再生速度の変更が論理上の位置と速度に効く", () => {
    vi.useFakeTimers();
    const { player, tech, middleware } = transcodePlayer();
    const controls = createPlayerControls(player, () => false);

    // 変換の開始位置 30 秒 + 技術層の 4 秒 = 論理上の 34 秒。
    expect(player.currentTime()).toBe(34);
    player.playbackRate(1.5);
    controls.seekBy(10);
    expect(middleware.currentTime(tech.currentTime())).toBe(44);

    vi.advanceTimersByTime(200);
    expect(tech.setSource).toHaveBeenCalledTimes(1);
    expect(tech.setSource.mock.calls[0]?.[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?startMs=44000",
      vvOffsetSeconds: 44,
    });
    // 作り直した変換でも、選んだ速度のまま再生を続ける。
    expect(tech.setPlaybackRate).toHaveBeenLastCalledWith(1.5);
  });

  it("buffer 済みの範囲への ← は同じ変換の中で戻る", () => {
    const { player, tech } = transcodePlayer();
    const controls = createPlayerControls(player, () => false);
    tech.buffered.mockReturnValue(ranges([[0, 20]]));
    controls.seekBy(-3);
    expect(tech.setCurrentTime).toHaveBeenLastCalledWith(1);
    expect(tech.setSource).not.toHaveBeenCalled();
  });

  it("先頭へ戻る操作は論理上の 0 秒へ移る", () => {
    vi.useFakeTimers();
    const { player, tech } = transcodePlayer();
    const controls = createPlayerControls(player, () => false);
    controls.seekTo(0);
    vi.advanceTimersByTime(200);
    expect(tech.setSource.mock.calls[0]?.[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4",
      vvOffsetSeconds: 0,
    });
  });
});
