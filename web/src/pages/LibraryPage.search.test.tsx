import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ListVideosParams, VideoPage } from "../api/client";

/**
 * 検索入力の待ち合わせと打ち切り（002 のまま変えない。FR-025 / SC-009）。
 *
 * TD-004 が名指しした 2 点目である。1 打鍵ごとに問い合わせると打っている間
 * ずっと一覧が入れ替わって読めない。かといって待ち合わせに取りこぼしがあると、
 * 打ち終えた語で引かれない。この 2 つは見た目からは確かめられない。
 *
 * 描画を MemoryRouter の中で行うのは VideoCard が Link を使うためである。
 * T015 で検索語を useSearchParams へ移したあとも、この書き方のまま通ること。
 */

const { listVideos, getCurrentScan, startScan } = vi.hoisted(() => ({
  listVideos: vi.fn(),
  getCurrentScan: vi.fn(),
  startScan: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  listVideos,
  getCurrentScan,
  startScan,
}));

const { default: LibraryPage } = await import("./LibraryPage");

/** searchDebounceMs は LibraryPage の待ち合わせ時間と同じ値である。 */
const searchDebounceMs = 250;

const emptyPage: VideoPage = { items: [], total: 0 };

/** queries は listVideos に渡された検索語を、呼ばれた順に返す。 */
function queries(): (string | undefined)[] {
  return listVideos.mock.calls.map((args: unknown[]) => {
    const params = args[0] as ListVideosParams | undefined;
    return params?.query;
  });
}

/** show は一覧を描き、最初の 1 ページ目の取得を終わらせる。 */
async function show(): Promise<void> {
  render(
    <MemoryRouter>
      <LibraryPage />
    </MemoryRouter>,
  );
  await act(async () => {
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.useFakeTimers();

  listVideos.mockReset();
  listVideos.mockResolvedValue(emptyPage);
  getCurrentScan.mockReset();
  getCurrentScan.mockResolvedValue(null);
  startScan.mockReset();

  // jsdom は交差の観測を持たない。続きの読み込みはこの検査の対象ではないので、
  // 何も観測しないものを置く。
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

describe("LibraryPage の検索（待ち合わせと打ち切り）", () => {
  it("打鍵のたびには問い合わせず、入力が止まってから 1 回だけ引く", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    await show();

    const initial = listVideos.mock.calls.length;
    expect(initial).toBe(1);

    await user.type(screen.getByPlaceholderText("題名で探す"), "ねこ");

    // 打っている間は増えない。
    expect(listVideos.mock.calls.length).toBe(initial);

    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs);
      await Promise.resolve();
    });

    expect(listVideos.mock.calls.length).toBe(initial + 1);
    expect(queries().at(-1)).toBe("ねこ");
  });

  it("打ち終えた値で必ず 1 回引く（取りこぼしが無い）", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    await show();

    await user.type(screen.getByPlaceholderText("題名で探す"), "いぬ");
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs);
      await Promise.resolve();
    });

    // 打ち終えた値がちょうど 1 回現れる。途中の語では引かない。
    expect(queries().filter((query) => query === "いぬ")).toHaveLength(1);
    expect(queries()).not.toContain("い");
  });

  it("待ち合わせ中に次の打鍵が来たら、前の待ちは破棄される", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    await show();

    const initial = listVideos.mock.calls.length;
    const field = screen.getByPlaceholderText("題名で探す");

    await user.type(field, "ね");
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs - 50);
      await Promise.resolve();
    });
    expect(listVideos.mock.calls.length).toBe(initial);

    await user.type(field, "こ");
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs - 50);
      await Promise.resolve();
    });
    // 前の待ちが生きていれば、ここで「ね」を引いてしまっている。
    expect(listVideos.mock.calls.length).toBe(initial);

    await act(async () => {
      vi.advanceTimersByTime(50);
      await Promise.resolve();
    });

    expect(listVideos.mock.calls.length).toBe(initial + 1);
    expect(queries().at(-1)).toBe("ねこ");
  });
});
