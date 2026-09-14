import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan, Video, VideoPage as VideoPageType } from "../api/client";

/**
 * 一覧 → 再生 → 一覧の往復（FR-016 / quickstart S5・S8）。
 *
 * 静止画では示せないところである。要点は 3 つある。
 *
 * 1. 戻り先が**離れたときの一覧**であること（`/` へ戻すと絞り込みが消える）
 * 2. 戻ったときに**読み直さない**こと（読み直すと 300 件まで読んだ状態が消える）
 * 3. 完了済みの取り込みが残っていても、それに巻き込まれて控えが捨てられないこと
 */

const { listVideos, getVideo, getCurrentScan, startScan, saveProgress, beaconProgress } =
  vi.hoisted(() => ({
    listVideos: vi.fn(),
    getVideo: vi.fn(),
    getCurrentScan: vi.fn(),
    startScan: vi.fn(),
    saveProgress: vi.fn(),
    beaconProgress: vi.fn(),
  }));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  listVideos,
  getVideo,
  getCurrentScan,
  startScan,
  saveProgress,
  beaconProgress,
}));

const { default: LibraryPage } = await import("./LibraryPage");
const { default: VideoPage } = await import("./VideoPage");
const { clearListSnapshot } = await import("../api/listSnapshot");

/** video は一覧に並べる 1 件を作る。 */
function video(id: number, title: string): Video {
  return {
    id,
    title,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
  };
}

/** finishedScan は「前回の取り込みが終わっている」状態である（ふつうの状態）。 */
const finishedScan: Scan = {
  id: 1,
  state: "done",
  total: 2,
  completed: 2,
  failed: 0,
  startedAt: "2026-09-13T00:00:00Z",
  finishedAt: "2026-09-13T00:00:01Z",
};

/** Here は現在の URL を読めるようにする（検査のためだけの部品）。 */
function Here() {
  const location = useLocation();
  return <output data-testid="here">{`${location.pathname}${location.search}`}</output>;
}

/** settle は描画直後の取得を終わらせる。 */
async function settle(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

/** show は一覧と再生の 2 画面を、往復できる形で描く。 */
function show(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/" element={<LibraryPage />} />
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
      <Here />
    </MemoryRouter>,
  );
}

/** here は現在の URL を返す。 */
function here(): string {
  return screen.getByTestId("here").textContent ?? "";
}

beforeEach(() => {
  clearListSnapshot();
  vi.useFakeTimers();

  listVideos.mockReset();
  listVideos.mockResolvedValue({
    items: [video(1, "ねこA.mp4"), video(2, "ねこB.mp4")],
    total: 2,
  } satisfies VideoPageType);

  getVideo.mockReset();
  getVideo.mockResolvedValue(video(1, "ねこA.mp4"));

  // 取り込みは完了済みで残っている。これがふつうの状態である。
  getCurrentScan.mockReset();
  getCurrentScan.mockResolvedValue(finishedScan);
  startScan.mockReset();
  saveProgress.mockReset();
  saveProgress.mockResolvedValue({
    positionMs: 0,
    completed: false,
    updatedAt: "2026-09-13T00:00:00Z",
  });
  beaconProgress.mockReset();
  beaconProgress.mockReturnValue(true);

  // jsdom は実際にはスクロールしない。復元が呼ぶこと自体はここでの主題では
  // ないので、警告だけ出させないようにする。
  vi.stubGlobal("scrollTo", vi.fn());

  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
      takeRecords() {
        return [];
      }
    },
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("一覧 → 再生 → 一覧 の往復（FR-016）", () => {
  it("絞り込んだ一覧へ戻り、読み直さない", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    show("/?q=%E3%81%AD%E3%81%93&sort=titleAsc");
    await settle();

    expect(listVideos.mock.calls).toHaveLength(1);

    await user.click(screen.getByRole("link", { name: "ねこA.mp4" }));
    await settle();
    expect(here()).toBe("/videos/1");

    await user.click(screen.getByRole("link", { name: "← 一覧へ戻る" }));
    await settle();

    // `/` へ戻すと検索語も並び順も消える。離れたときの一覧に帰ること。
    expect(here()).toBe("/?q=%E3%81%AD%E3%81%93&sort=titleAsc");
    expect(screen.getByRole("link", { name: "ねこA.mp4" })).toBeDefined();

    // 控えから戻すので、取得は増えない。完了済みの取り込みが残っていても、
    // それに巻き込まれて控えが捨てられない。
    expect(listVideos.mock.calls).toHaveLength(1);
  });

  it("直接開いた再生画面からは一覧の先頭へ戻る", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    show("/videos/1");
    await settle();

    await user.click(screen.getByRole("link", { name: "← 一覧へ戻る" }));
    await settle();

    // 遷移元が無いので、戻り先は既定の一覧である。
    expect(here()).toBe("/");
  });
});
