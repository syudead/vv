import { act, render } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";

/**
 * 再生位置の送信（002 のまま変えない。FR-025 / SC-009）。
 *
 * TD-004 が名指しした 3 点目である。5 秒ごとの送信、同じ位置を送らないこと、
 * 離脱時の sendBeacon は、いずれも画面を見ていても分からない。004 で
 * VideoPage の並びを組み替える前に固定しておく。
 */

const { getVideo, saveProgress, beaconProgress } = vi.hoisted(() => ({
  getVideo: vi.fn(),
  saveProgress: vi.fn(),
  beaconProgress: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  getVideo,
  saveProgress,
  beaconProgress,
}));

const { default: VideoPage } = await import("./VideoPage");

/** saveIntervalMs は VideoPage の送信間隔と同じ値である（R-111）。 */
const saveIntervalMs = 5000;

const video: Video = {
  id: 7,
  title: "長い動画",
  sizeBytes: 1024,
  addedAt: "2026-09-13T00:00:00Z",
  durationMs: 600_000,
  playable: true,
  probeState: "done",
  thumbnailState: "done",
};

/** playing は jsdom の <video> に再生中のふるまいを持たせる。 */
function playing(): { seek: (seconds: number) => void } {
  const element = document.querySelector("video");
  if (element === null) {
    throw new Error("映像が描かれていない");
  }

  let currentTime = 0;
  Object.defineProperty(element, "currentTime", {
    configurable: true,
    get: () => currentTime,
    set: (value: number) => {
      currentTime = value;
    },
  });
  Object.defineProperty(element, "paused", { configurable: true, value: false });
  Object.defineProperty(element, "ended", { configurable: true, value: false });

  return {
    seek: (seconds) => {
      currentTime = seconds;
    },
  };
}

/** show は再生画面を描き、詳細の取得を終わらせる。 */
async function show(): Promise<() => void> {
  const { unmount } = render(
    <MemoryRouter initialEntries={["/videos/7"]}>
      <Routes>
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </MemoryRouter>,
  );
  await act(async () => {
    await Promise.resolve();
  });
  return unmount;
}

/** tick は間隔 1 回ぶん時間を進める。 */
async function tick(times = 1): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(saveIntervalMs * times);
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.useFakeTimers();

  getVideo.mockReset();
  getVideo.mockResolvedValue(video);
  saveProgress.mockReset();
  saveProgress.mockResolvedValue({
    positionMs: 0,
    completed: false,
    updatedAt: "2026-09-13T00:00:00Z",
  });
  beaconProgress.mockReset();
  beaconProgress.mockReturnValue(true);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("VideoPage の再生位置の送信", () => {
  it("再生中は 5 秒ごとに位置を送る", async () => {
    await show();
    const { seek } = playing();

    seek(12);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(1);
    expect(saveProgress.mock.calls[0]).toEqual([7, 12_000]);

    seek(17);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(2);
    expect(saveProgress.mock.calls[1]).toEqual([7, 17_000]);
  });

  it("同じ位置を繰り返し送らない", async () => {
    await show();
    const { seek } = playing();

    seek(12);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(1);

    // 位置が動いていない（一時停止・停滞）。送っても書き込みが増えるだけである。
    await tick(3);
    expect(saveProgress.mock.calls).toHaveLength(1);

    // 1 秒に満たない差も送らない。
    seek(12.5);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(1);

    seek(14);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(2);
  });

  it("画面が隠れたら sendBeacon で最後の位置を送る", async () => {
    await show();
    const { seek } = playing();

    seek(31);
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });

    expect(beaconProgress.mock.calls).toHaveLength(1);
    expect(beaconProgress.mock.calls[0]).toEqual([7, 31_000]);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
  });

  // 既知の欠落（TD-008）。VideoPage の後片付けは videoRef.current を読むが、
  // React は参照を外してから useEffect の後片付けを呼ぶので、離脱時の経路は
  // その時点で必ず null になっている。**いまの振る舞い**をそのまま書き留めて
  // おく — 本機能（004）は既存の振る舞いを変えないことが要求（FR-025）なので、
  // 直すのは VideoPage を組み替える T024〜T026 の側である。
  // 直したらこの検査が落ちるので、そのとき期待を「送られる」へ入れ替えること。
  it("いまは画面を離れるとき（unmount）には送られない（TD-008）", async () => {
    const unmount = await show();
    const { seek } = playing();

    seek(42);
    await act(async () => {
      unmount();
      await Promise.resolve();
    });

    expect(beaconProgress.mock.calls).toHaveLength(0);
  });

  it("送信に失敗しても例外が外へ出ず、次の送信で追いつく", async () => {
    saveProgress.mockRejectedValue(new Error("通信できません"));

    await show();
    const { seek } = playing();

    seek(12);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(1);

    // 失敗しても再生は続く。次の位置がそのまま送られる。
    saveProgress.mockResolvedValue({
      positionMs: 20_000,
      completed: false,
      updatedAt: "2026-09-13T00:00:00Z",
    });
    seek(20);
    await tick();
    expect(saveProgress.mock.calls).toHaveLength(2);
    expect(saveProgress.mock.calls[1]).toEqual([7, 20_000]);
  });
});
