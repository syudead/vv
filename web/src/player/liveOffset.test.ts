import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getTranscodeStart } from "../api/client";
import { createLiveOffsetMiddleware, liveSource } from "./liveOffset";

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  getTranscodeStart: vi.fn(),
}));

const transcodeStart = vi.mocked(getTranscodeStart);

/** report は実際の開始位置の報告を、テストが決めるまで保留する。 */
function deferredReports() {
  const pending: {
    attempt: string;
    resolve: (startMs: number) => void;
    reject: (error: Error) => void;
  }[] = [];
  transcodeStart.mockImplementation(
    (_id, attempt) =>
      new Promise<number>((resolve, reject) => {
        pending.push({ attempt, resolve, reject });
      }),
  );
  return pending;
}

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
  beforeEach(() => {
    // 既定では報告は届かず、指定位置を表示し続ける（404 と同じ）。
    transcodeStart.mockReset();
    transcodeStart.mockRejectedValue(new Error("not found"));
  });
  afterEach(() => vi.useRealTimers());

  it("duration、currentTime、bufferedを元動画の時間軸へ写す", async () => {
    const { middleware } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000), () => undefined);
    await vi.waitFor(() => expect(middleware.currentTime(5)).toBe(35));

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
    const reloaded = tech.setSource.mock.calls[0]?.[0] as { src: string };
    expect(reloaded).toMatchObject({ vvOffsetSeconds: 70 });
    expect(reloaded.src).toMatch(
      /^\/api\/videos\/7\/transcode\.mp4\?startMs=70000&attempt=[0-9a-f]{32}$/,
    );
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

  it("途中から始めるsourceだけにattemptを付ける", () => {
    const fromStart = liveSource(7, 120_000, 0);
    expect(fromStart.src).toBe("/api/videos/7/transcode.mp4");
    expect(fromStart.vvAttempt).toBeUndefined();

    const first = liveSource(7, 120_000, 30_000);
    const second = liveSource(7, 120_000, 30_000);
    expect(first.vvAttempt).toMatch(/^[A-Za-z0-9_-]{1,64}$/);
    expect(new URL(first.src, "http://localhost").searchParams.get("attempt")).toBe(
      first.vvAttempt,
    );
    expect(second.vvAttempt).not.toBe(first.vvAttempt);
  });

  it("報告が届くまで指定位置を返し、届いたら実際の開始位置へ置き換える", async () => {
    const reports = deferredReports();
    const changed = vi.fn();
    const { middleware, tech } = fixture();
    const source = liveSource(7, 120_000, 30_000, changed);
    middleware.setSource(source, () => undefined);

    expect(transcodeStart).toHaveBeenCalledWith(7, source.vvAttempt, expect.anything());
    expect(middleware.currentTime(0)).toBe(30);
    expect(middleware.currentTime(2)).toBe(30);

    reports[0]?.resolve(22_500);
    await vi.waitFor(() => expect(middleware.currentTime(2)).toBe(24.5));
    expect(changed).toHaveBeenLastCalledWith(22.5);
    const shifted = middleware.buffered(ranges([[0, 10]]));
    expect([shifted.start(0), shifted.end(0)]).toEqual([22.5, 32.5]);
    expect(tech.trigger).toHaveBeenCalledWith("timeupdate");
  });

  it("報告が404か誤りなら指定位置を表示し続ける", async () => {
    const reports = deferredReports();
    const changed = vi.fn();
    const { middleware } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000, changed), () => undefined);

    reports[0]?.reject(new Error("404"));
    await vi.waitFor(() => expect(middleware.currentTime(2)).toBe(32));
    expect(changed).not.toHaveBeenCalled();
  });

  it("未buffer seekのreloadでも新しいattemptの報告で位置を合わせる", async () => {
    vi.useFakeTimers();
    const reports = deferredReports();
    const changed = vi.fn();
    const { middleware, tech } = fixture();
    middleware.setSource(liveSource(7, 120_000, 0, changed), () => undefined);
    expect(transcodeStart).not.toHaveBeenCalled();

    middleware.setCurrentTime(70);
    vi.advanceTimersByTime(200);
    const reloaded = tech.setSource.mock.calls[0]?.[0] as { vvAttempt: string };
    expect(reports[0]?.attempt).toBe(reloaded.vvAttempt);
    expect(middleware.currentTime(1)).toBe(70);

    reports[0]?.resolve(62_000);
    await vi.waitFor(() => expect(middleware.currentTime(1)).toBe(63));
    expect(changed).toHaveBeenLastCalledWith(62);
  });

  it("sourceを差し替えたあとに届いた古いattemptの報告は捨てる", async () => {
    vi.useFakeTimers();
    const reports = deferredReports();
    const changed = vi.fn();
    const { middleware, tech } = fixture();
    tech.buffered.mockReturnValue(ranges([]));
    middleware.setSource(liveSource(7, 120_000, 30_000, changed), () => undefined);

    middleware.setCurrentTime(80);
    vi.advanceTimersByTime(200);
    expect(reports).toHaveLength(2);

    reports[0]?.resolve(20_000);
    await vi.advanceTimersByTimeAsync(0);
    expect(middleware.currentTime(0)).toBe(80);
    expect(changed).not.toHaveBeenCalledWith(20);

    reports[1]?.resolve(75_000);
    await vi.waitFor(() => expect(middleware.currentTime(0)).toBe(75));
  });

  it("disposeのあとに届いた報告は位置を変えない", async () => {
    const reports = deferredReports();
    const changed = vi.fn();
    const { middleware, dispose } = fixture();
    middleware.setSource(liveSource(7, 120_000, 30_000, changed), () => undefined);
    dispose();
    reports[0]?.resolve(20_000);
    await Promise.resolve();
    await Promise.resolve();
    expect(changed).not.toHaveBeenCalled();
  });
});
