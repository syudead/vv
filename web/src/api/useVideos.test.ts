import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ListVideosParams, Video, VideoPage, VideoSort } from "./client";

/**
 * 一覧の読み込みの振る舞いを固定する。
 *
 * カーソルの引き継ぎと打ち切りは目で見て分かる形に現れない。画面を書き換えても
 * **このテストが同じ内容で通ること**が、既存の振る舞いを保った証拠になる。
 */

const { listVideos } = vi.hoisted(() => ({ listVideos: vi.fn() }));

vi.mock("./client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./client")>()),
  listVideos,
}));

const { useVideos } = await import("./useVideos");
const { nextProgressSequence, recordSavedProgress } = await import("./progressEvents");

/** Pending は応答を後から決められる 1 回の呼び出しである。 */
interface Pending {
  params: ListVideosParams;
  resolve: (page: VideoPage) => void;
  reject: (reason: unknown) => void;
  aborted: () => boolean;
}

let calls: Pending[] = [];

/** item は一覧の 1 件を作る。 */
function item(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
  };
}

/** page は 1 ページ分の応答を作る。 */
function page(ids: number[], nextCursor?: string): VideoPage {
  return { items: ids.map(item), total: 100, nextCursor };
}

beforeEach(() => {
  calls = [];
  listVideos.mockReset();
  listVideos.mockImplementation((params: ListVideosParams = {}) => {
    let settle: ((value: VideoPage) => void) | null = null;
    let fail: ((reason: unknown) => void) | null = null;
    const promise = new Promise<VideoPage>((res, rej) => {
      settle = res;
      fail = rej;
    });

    // 本物の fetch と同じく、打ち切られた要求は AbortError で終わる。
    params.signal?.addEventListener("abort", () => {
      fail?.(new DOMException("aborted", "AbortError"));
    });

    calls.push({
      params,
      resolve: (value) => settle?.(value),
      reject: (reason) => fail?.(reason),
      aborted: () => params.signal?.aborted === true,
    });
    return promise;
  });
});

describe("useVideos（一覧の読み込み）", () => {
  it("loadMore は直前の応答の nextCursor をそのまま次の要求へ渡す", async () => {
    const { result } = renderHook(() => useVideos("addedDesc", ""));

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    expect(calls[0]?.params.cursor).toBeUndefined();

    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    expect(result.current.items.map((video) => video.id)).toEqual([1, 2]);
    expect(result.current.hasMore).toBe(true);

    act(() => {
      result.current.loadMore();
    });

    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[1]?.params.cursor).toBe("cursor-1");

    // 続きは置き換えではなく連結である。
    await act(async () => {
      calls[1]?.resolve(page([3], "cursor-2"));
    });
    expect(result.current.items.map((video) => video.id)).toEqual([1, 2, 3]);
  });

  it("保存された再生位置を、表示中の該当する項目にだけ反映する", async () => {
    const { result } = renderHook(() => useVideos("addedDesc", ""));
    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2]));
    });

    const progress = {
      positionMs: 60_000,
      completed: true,
      updatedAt: "2026-09-23T00:00:00Z",
    };
    act(() => {
      recordSavedProgress(2, progress, nextProgressSequence());
    });

    expect(result.current.items.find((video) => video.id === 2)?.progress).toEqual(
      progress,
    );
    expect(
      result.current.items.find((video) => video.id === 1)?.progress,
    ).toBeUndefined();
  });

  it("nextCursor が無ければ hasMore が偽になり、loadMore は要求を出さない", async () => {
    const { result } = renderHook(() => useVideos("addedDesc", ""));

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2]));
    });

    expect(result.current.hasMore).toBe(false);

    act(() => {
      result.current.loadMore();
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(calls).toHaveLength(1);
  });

  it("続きの取得失敗後に同じカーソルから再試行できる", async () => {
    const { result } = renderHook(() => useVideos("addedDesc", ""));

    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "cursor-1")));
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));

    await act(async () => calls[1]?.reject(new Error("一時的な失敗")));
    expect(result.current.error).toBe("一時的な失敗");
    expect(result.current.hasMore).toBe(false);

    act(() => result.current.retryLoadMore());
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBe("cursor-1");
    await act(async () => calls[2]?.resolve(page([3])));
    expect(result.current.items.map((video) => video.id)).toEqual([1, 2, 3]);
    expect(result.current.error).toBeNull();
  });

  it("並び順や検索語が変わったらカーソルを引き継がず先頭から読み直す", async () => {
    const { result, rerender } = renderHook(
      ({ sort, query }: { sort: VideoSort; query: string }) => useVideos(sort, query),
      { initialProps: { sort: "addedDesc" as VideoSort, query: "" } },
    );

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });

    // カーソルは並び順と検索語に紐づく。引き継ぐと境界の意味が変わってしまう。
    rerender({ sort: "titleAsc", query: "" });
    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[1]?.params.cursor).toBeUndefined();
    expect(calls[1]?.params.sort).toBe("titleAsc");

    await act(async () => {
      calls[1]?.resolve(page([7], "cursor-9"));
    });
    expect(result.current.items.map((video) => video.id)).toEqual([7]);

    rerender({ sort: "titleAsc", query: "ねこ" });
    await waitFor(() => {
      expect(calls).toHaveLength(3);
    });
    expect(calls[2]?.params.cursor).toBeUndefined();
    expect(calls[2]?.params.query).toBe("ねこ");
  });

  it("連続して変えたとき、古い応答が新しい一覧を上書きしない", async () => {
    const { result, rerender } = renderHook(
      ({ query }: { query: string }) => useVideos("addedDesc", query),
      { initialProps: { query: "ね" } },
    );

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });

    // 1 つ目が返る前に検索語が変わる。
    rerender({ query: "ねこ" });
    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[0]?.aborted()).toBe(true);

    await act(async () => {
      calls[1]?.resolve(page([42], "cursor-1"));
    });

    expect(result.current.items.map((video) => video.id)).toEqual([42]);
    // 打ち切りは「利用者が先に進んだ」だけで、失敗ではない。
    expect(result.current.error).toBeNull();
    expect(result.current.hasMore).toBe(true);
    expect(result.current.loading).toBe(false);
  });
});
