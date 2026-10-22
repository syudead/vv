import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OwnerAudience } from "../testing/audience";
import type {
  LibraryGroup,
  LibraryItem,
  ListVideosParams,
  Video,
  VideoPage,
  VideoSort,
} from "./client";
import {
  emitServerEvent,
  FakeEventSource,
  installFakeEventSource,
} from "./fakeEventSource";
import { itemVideos, videoItem } from "./libraryItems";
import type { VideosCriteria } from "./useVideos";

const { listVideos, listLibrary, listFolderVideos, getVideo, getGroup } = vi.hoisted(
  () => ({
    listVideos: vi.fn(),
    listLibrary: vi.fn(),
    listFolderVideos: vi.fn(),
    getVideo: vi.fn(),
    getGroup: vi.fn(),
  }),
);

vi.mock("./client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./client")>()),
  listVideos,
  listLibrary,
  listFolderVideos,
  getVideo,
  getGroup,
}));

const { useVideos } = await import("./useVideos");
const { nextProgressSequence, recordSavedProgress } = await import("./progressEvents");

interface Pending {
  params: ListVideosParams;
  resolve: (page: VideoPage) => void;
  reject: (reason: unknown) => void;
  aborted: () => boolean;
}

let calls: Pending[] = [];

function item(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    updatedAt: "2026-09-13T00:00:00Z",
    fileCreatedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

function page(ids: number[], nextCursor?: string): VideoPage {
  return { items: ids.map(item), total: 100, nextCursor };
}

beforeEach(() => {
  calls = [];
  installFakeEventSource();
  getVideo.mockReset();
  getGroup.mockReset();
  listLibrary.mockReset();
  listVideos.mockReset();
  listVideos.mockImplementation((params: ListVideosParams = {}) => {
    let settle: ((value: VideoPage) => void) | null = null;
    let fail: ((reason: unknown) => void) | null = null;
    const promise = new Promise<VideoPage>((res, rej) => {
      settle = res;
      fail = rej;
    });

    // Match fetch cancellation with AbortError.
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
  it("ゲストは変化の知らせ（/api/events）のうち group だけを受け取る", async () => {
    // The default audience outside AudienceProvider is guest.
    renderHook(() => useVideos({ sort: "addedDesc" }));
    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2]));
    });
    expect(FakeEventSource.instances).toHaveLength(1);
    await emitServerEvent("video", { id: 2 });
    expect(getVideo).not.toHaveBeenCalled();
  });

  it("loadMore は直前の応答の nextCursor をそのまま次の要求へ渡す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    expect(calls[0]?.params.cursor).toBeUndefined();

    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2]);
    expect(result.current.hasMore).toBe(true);

    act(() => {
      result.current.loadMore();
    });

    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[1]?.params.cursor).toBe("cursor-1");

    await act(async () => {
      calls[1]?.resolve(page([3], "cursor-2"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2, 3]);
  });

  it("refreshInPlace は読み込み済みの件数まで取り直し、一覧を消さずに置き換える", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      calls[1]?.resolve(page([3, 4], "cursor-2"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4,
    ]);

    act(() => result.current.refreshInPlace());
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBeUndefined();
    expect(result.current.loading).toBe(false);
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4,
    ]);

    await act(async () => {
      calls[2]?.resolve(page([9, 1, 2], "cursor-a"));
    });
    await waitFor(() => expect(calls).toHaveLength(4));
    expect(calls[3]?.params.cursor).toBe("cursor-a");
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4,
    ]);
    await act(async () => {
      calls[3]?.resolve(page([3, 4], "cursor-b"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      9, 1, 2, 3, 4,
    ]);
    expect(result.current.cursor).toBe("cursor-b");
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("refreshInPlace の失敗は静かに捨てて今の一覧を残す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });

    act(() => result.current.refreshInPlace());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      calls[1]?.reject(new Error("offline"));
    });

    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2]);
    expect(result.current.error).toBeNull();
    expect(result.current.hasMore).toBe(true);
  });

  it("取り直しの途中の loadMore と retryLoadMore は取り直しを打ち切らず、終わってから続きを取る", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });

    act(() => result.current.refreshInPlace());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => result.current.loadMore());
    act(() => result.current.retryLoadMore());
    expect(calls).toHaveLength(2);

    await act(async () => {
      calls[1]?.resolve(page([9, 1], "cursor-2"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([9, 1]);
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBe("cursor-2");
  });

  it("取り直しは、ページの境で重なった項目を数えずに件数に届くまで続きを取る", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      calls[1]?.resolve(page([3, 4], "cursor-2"));
    });

    act(() => result.current.refreshInPlace());
    await waitFor(() => expect(calls).toHaveLength(3));
    await act(async () => {
      calls[2]?.resolve(page([1, 2], "cursor-a"));
    });
    await waitFor(() => expect(calls).toHaveLength(4));
    await act(async () => {
      calls[3]?.resolve(page([2, 3], "cursor-b"));
    });
    await waitFor(() => expect(calls).toHaveLength(5));
    expect(calls[4]?.params.cursor).toBe("cursor-b");
    await act(async () => {
      calls[4]?.resolve(page([4, 5], "cursor-c"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4, 5,
    ]);
    expect(result.current.cursor).toBe("cursor-c");
  });

  it("ページの取得の途中の refreshInPlace は、その取得が終わってから取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));

    act(() => result.current.refreshInPlace());
    expect(calls).toHaveLength(1);

    await act(async () => {
      calls[0]?.resolve(page([1, 2]));
    });
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1]?.params.cursor).toBeUndefined();
  });

  it("続きの取得の途中の refreshInPlace は、続きを加えた件数まで取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));

    act(() => result.current.refreshInPlace());
    expect(calls).toHaveLength(2);

    await act(async () => {
      calls[1]?.resolve(page([3, 4], "cursor-2"));
    });
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBeUndefined();

    await act(async () => {
      calls[2]?.resolve(page([1, 2], "cursor-1"));
    });
    await waitFor(() => expect(calls).toHaveLength(4));
    expect(calls[3]?.params.cursor).toBe("cursor-1");
    await act(async () => {
      calls[3]?.resolve(page([3, 4], "cursor-2"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4,
    ]);
  });

  it("続きのページが既に出ている項目と重なっても、取り直しは実際の件数で止まる", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => result.current.refreshInPlace());

    await act(async () => {
      calls[1]?.resolve(page([2, 3], "cursor-2"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2, 3]);
    await waitFor(() => expect(calls).toHaveLength(3));
    await act(async () => {
      calls[2]?.resolve(page([1, 2], "cursor-1"));
    });
    await waitFor(() => expect(calls).toHaveLength(4));
    await act(async () => {
      calls[3]?.resolve(page([3], "cursor-2"));
    });

    expect(calls).toHaveLength(4);
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2, 3]);
  });

  it("続きの応答で total がカード数を下回ったら先頭から読み直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

    await act(async () => {
      calls[0]?.resolve({ items: [item(1), item(2)], total: 3, nextCursor: "next" });
    });
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));

    await act(async () => {
      calls[1]?.resolve({ items: [item(3)], total: 2 });
    });
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBeUndefined();

    await act(async () => {
      calls[2]?.resolve({ items: [item(2), item(3)], total: 2 });
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([2, 3]);
    expect(result.current.total).toBe(2);
  });

  it("先頭ページで total がカード数を下回ったら1度だけ読み直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

    await act(async () => {
      calls[0]?.resolve({ items: [item(1), item(2)], total: 1 });
    });
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1]?.params.cursor).toBeUndefined();
    expect(result.current.loading).toBe(true);

    await act(async () => {
      calls[1]?.resolve({ items: [item(1), item(2)], total: 2 });
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2]);
    expect(result.current.total).toBe(2);
  });

  it("先頭ページの矛盾が続いても再読込を繰り返さない", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

    await act(async () => {
      calls[0]?.resolve({ items: [item(1), item(2)], total: 1 });
    });
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      calls[1]?.resolve({ items: [item(1), item(2)], total: 1 });
    });

    expect(itemVideos(result.current.items)).toEqual([]);
    expect(result.current.error).toContain("Try again in a moment");
    expect(calls).toHaveLength(2);
  });

  it("続きの応答と同じ描画に入った再生位置の更新を保持する", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    const progress = {
      positionMs: 30_000,
      completed: false,
      updatedAt: "2026-09-25T00:00:00Z",
    };

    await act(async () => {
      recordSavedProgress(2, progress, nextProgressSequence());
      calls[1]?.resolve(page([3]));
    });

    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2, 3]);
    expect(itemVideos(result.current.items)[1]?.progress).toEqual(progress);
  });

  it("先頭ページの再試行中は前のエラーを消す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.reject(new Error("一時的な失敗")));
    expect(result.current.error).toBe("Something went wrong.");

    act(() => result.current.reload());
    await waitFor(() => expect(calls).toHaveLength(2));

    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("保存された再生位置を、表示中の該当する項目にだけ反映する", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
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

    expect(
      itemVideos(result.current.items).find((video) => video.id === 2)?.progress,
    ).toEqual(progress);
    expect(
      itemVideos(result.current.items).find((video) => video.id === 1)?.progress,
    ).toBeUndefined();
  });

  it("nextCursor が無ければ hasMore が偽になり、loadMore は要求を出さない", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

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
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });

    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "cursor-1")));
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));

    await act(async () => calls[1]?.reject(new Error("一時的な失敗")));
    expect(result.current.error).toBe("Something went wrong.");
    expect(result.current.hasMore).toBe(false);

    act(() => result.current.retryLoadMore());
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params.cursor).toBe("cursor-1");
    await act(async () => calls[2]?.resolve(page([3])));
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2, 3]);
    expect(result.current.error).toBeNull();
  });

  it("並び順や検索語が変わったらカーソルを引き継がず先頭から読み直す", async () => {
    const { result, rerender } = renderHook(
      ({ sort, query }: { sort: VideoSort; query: string }) => useVideos({ sort, query }),
      {
        initialProps: { sort: "addedDesc" as VideoSort, query: "" },
        wrapper: OwnerAudience,
      },
    );

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    await act(async () => {
      calls[0]?.resolve(page([1, 2], "cursor-1"));
    });

    // Cursors belong to their sort and query; reusing them changes the boundary's meaning.
    rerender({ sort: "titleAsc", query: "" });
    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[1]?.params.cursor).toBeUndefined();
    expect(calls[1]?.params.sort).toBe("titleAsc");

    await act(async () => {
      calls[1]?.resolve(page([7], "cursor-9"));
    });
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([7]);

    rerender({ sort: "titleAsc", query: "ねこ" });
    await waitFor(() => {
      expect(calls).toHaveLength(3);
    });
    expect(calls[2]?.params.cursor).toBeUndefined();
    expect(calls[2]?.params.query).toBe("ねこ");
  });

  it("連続して変えたとき、古い応答が新しい一覧を上書きしない", async () => {
    const { result, rerender } = renderHook(
      ({ query }: { query: string }) => useVideos({ sort: "addedDesc", query }),
      { initialProps: { query: "ね" }, wrapper: OwnerAudience },
    );

    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });

    rerender({ query: "ねこ" });
    await waitFor(() => {
      expect(calls).toHaveLength(2);
    });
    expect(calls[0]?.aborted()).toBe(true);

    await act(async () => {
      calls[1]?.resolve(page([42], "cursor-1"));
    });

    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([42]);
    expect(result.current.error).toBeNull();
    expect(result.current.hasMore).toBe(true);
    expect(result.current.loading).toBe(false);
  });
});

describe("useVideos（条件と重複）", () => {
  it("条件（検索語・お気に入り・並び順・seed）をそのまま要求へ渡す", async () => {
    renderHook(() =>
      useVideos({
        query: "京都",
        favorite: true,
        sort: "random",
        seed: 42,
      }),
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]?.params).toMatchObject({
      query: "京都",
      favorite: true,
      sort: "random",
      seed: 42,
    });
  });

  it("random 以外の並び順では seed を送らない", async () => {
    renderHook(() => useVideos({ sort: "titleAsc", seed: 42 }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]?.params.seed).toBeUndefined();
  });

  const changes: [string, VideosCriteria][] = [
    ["検索語", { sort: "addedDesc", query: "奈良" }],
    ["お気に入り", { sort: "addedDesc", favorite: true }],
    ["並び順", { sort: "durationDesc" }],
    ["seed", { sort: "random", seed: 8 }],
  ];

  it.each(changes)(
    "要求の途中で%sを変えると、遅れて届いた古い応答を捨てる",
    async (_, next) => {
      const first: VideosCriteria =
        next.sort === "random" ? { sort: "random", seed: 7 } : { sort: "addedDesc" };
      const { result, rerender } = renderHook(
        ({ criteria }: { criteria: VideosCriteria }) => useVideos(criteria),
        { initialProps: { criteria: first }, wrapper: OwnerAudience },
      );
      await waitFor(() => expect(calls).toHaveLength(1));

      rerender({ criteria: next });
      await waitFor(() => expect(calls).toHaveLength(2));
      expect(calls[0]?.aborted()).toBe(true);

      await act(async () => calls[1]?.resolve(page([9])));
      await act(async () => calls[0]?.resolve(page([1, 2, 3])));
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([9]);
      expect(result.current.error).toBeNull();
    },
  );

  it("同じ条件で描画し直しても読み直さない", async () => {
    const { rerender } = renderHook(
      ({ criteria }: { criteria: VideosCriteria }) => useVideos(criteria),
      {
        initialProps: { criteria: { sort: "addedDesc", query: "a" } as VideosCriteria },
        wrapper: OwnerAudience,
      },
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    rerender({ criteria: { sort: "addedDesc", query: "a" } });
    await act(async () => Promise.resolve());
    expect(calls).toHaveLength(1);
  });

  it("条件を変えると1回だけ読み直し、そのあとの続きの取得は新しい条件で読む", async () => {
    const { result, rerender } = renderHook(
      ({ criteria }: { criteria: VideosCriteria }) => useVideos(criteria),
      {
        initialProps: { criteria: { sort: "addedDesc", query: "a" } as VideosCriteria },
        wrapper: OwnerAudience,
      },
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1], "cursor-a")));

    rerender({ criteria: { sort: "addedDesc", query: "b" } });
    await waitFor(() => expect(calls).toHaveLength(2));
    rerender({ criteria: { sort: "addedDesc", query: "b" } });
    await act(async () => calls[1]?.resolve(page([2], "cursor-b")));
    expect(calls).toHaveLength(2);
    expect(calls[1]?.params).toMatchObject({ query: "b", cursor: undefined });

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(3));
    expect(calls[2]?.params).toMatchObject({ query: "b", cursor: "cursor-b" });
  });

  it("続きのページに既に出た id が含まれていても、一覧に2度出さない", async () => {
    const { result } = renderHook(() => useVideos({ sort: "sizeDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3], "cursor-1")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => calls[1]?.resolve(page([3, 4, 2, 5, 5])));

    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([
      1, 2, 3, 4, 5,
    ]);
  });
});

describe("useVideos の準備の反映", () => {
  it("動画が変わった知らせで、その項目だけを取り直して置き換える", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));
    expect(itemVideos(result.current.items)).toHaveLength(2);

    getVideo.mockResolvedValue({
      ...item(2),
      title: "代表の所在の題名",
      previewState: "done",
      durationMs: 5000,
    });
    await emitServerEvent("video", { id: 2 });
    await waitFor(() =>
      expect(itemVideos(result.current.items)[1]?.previewState).toBe("done"),
    );

    expect(getVideo).toHaveBeenCalledTimes(1);
    expect(getVideo.mock.calls[0]?.[0]).toBe(2);
    expect(listVideos).toHaveBeenCalledTimes(1);
    expect(result.current.cursor).toBe("next");
    expect(itemVideos(result.current.items)[1]).toMatchObject({
      durationMs: 5000,
      // Folder listings use the title of the location in that folder, not necessarily the detail title.
      title: "動画 2",
    });
  });

  it("条件を変えて読み直したら、前の一覧のために始めた取り直しの応答を重ねない", async () => {
    const { result, rerender } = renderHook(
      ({ criteria }: { criteria: VideosCriteria }) => useVideos(criteria),
      {
        initialProps: { criteria: { sort: "addedDesc" } as VideosCriteria },
        wrapper: OwnerAudience,
      },
    );
    await act(async () => calls[0]?.resolve(page([1, 2])));

    let resolveOld: (video: Video) => void = () => {};
    getVideo.mockImplementationOnce(
      () =>
        new Promise<Video>((resolve) => {
          resolveOld = resolve;
        }),
    );
    await emitServerEvent("video", { id: 2 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(1));

    rerender({ criteria: { sort: "addedDesc", query: "新" } });
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () =>
      calls[1]?.resolve({ items: [{ ...item(2), previewState: "done" }], total: 1 }),
    );
    expect(itemVideos(result.current.items)[0]?.previewState).toBe("done");

    await act(async () => resolveOld({ ...item(2), previewState: "pending" }));
    expect(itemVideos(result.current.items)[0]?.previewState).toBe("done");
  });

  it("条件を変えた後に古い要求が 404 で失敗しても、新しい一覧を見つからない扱いにしない", async () => {
    let rejectOld: (reason: unknown) => void = () => {};
    listFolderVideos.mockImplementationOnce(
      () =>
        new Promise<VideoPage>((_, reject) => {
          rejectOld = reject;
        }),
    );
    listFolderVideos.mockImplementationOnce(() =>
      Promise.resolve({ items: [item(5)], total: 1 }),
    );
    const folder = { rootId: 3, path: "A" };
    const { result, rerender } = renderHook(
      ({ criteria }: { criteria: VideosCriteria }) =>
        useVideos(criteria, undefined, folder),
      {
        initialProps: { criteria: { sort: "addedDesc" } as VideosCriteria },
        wrapper: OwnerAudience,
      },
    );
    await waitFor(() => expect(listFolderVideos).toHaveBeenCalledTimes(1));

    rerender({ criteria: { sort: "addedDesc", query: "新", scope: "subtree" } });
    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([5]),
    );

    const { RequestFailed } = await import("./client");
    await act(async () =>
      rejectOld(new RequestFailed(404, "not_found", "見つかりません")),
    );
    expect(result.current.notFound).toBe(false);
    expect(result.current.error).toBeNull();
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([5]);
  });

  it("一覧に無い動画の知らせでは取りに行かない", async () => {
    renderHook(() => useVideos({ sort: "addedDesc" }), { wrapper: OwnerAudience });
    await act(async () => calls[0]?.resolve(page([1])));

    await emitServerEvent("video", { id: 99 });

    expect(getVideo).not.toHaveBeenCalled();
  });

  it("最初につながったら、準備中の項目だけを取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () =>
      calls[0]?.resolve({
        items: [{ ...item(1), previewState: "done" }, item(2)],
        total: 2,
      }),
    );
    getVideo.mockResolvedValue({ ...item(2), previewState: "done" });

    await emitServerEvent("open");

    await waitFor(() =>
      expect(itemVideos(result.current.items)[1]?.previewState).toBe("done"),
    );
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([2]);
  });

  it("つなぎ直したら、切れていた間に消えた準備済みの動画も一覧から外す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () =>
      calls[0]?.resolve({
        items: [
          { ...item(1), previewState: "done" },
          { ...item(2), previewState: "done" },
        ],
        total: 2,
      }),
    );
    await emitServerEvent("open");
    expect(getVideo).not.toHaveBeenCalled();

    const { RequestFailed } = await import("./client");
    getVideo.mockImplementation((id: number) =>
      id === 2
        ? Promise.reject(new RequestFailed(404, "not_found", "見つかりません"))
        : Promise.resolve({ ...item(1), previewState: "done" }),
    );
    await emitServerEvent("open");

    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1]),
    );
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([1, 2]);
  });

  it("ページの取得中に届いた知らせは、ページを反映したあとで取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    getVideo.mockResolvedValue({ ...item(2), previewState: "done" });

    await emitServerEvent("video", { id: 2 });
    expect(getVideo).not.toHaveBeenCalled();

    await act(async () => calls[0]?.resolve(page([1, 2])));

    await waitFor(() =>
      expect(itemVideos(result.current.items)[1]?.previewState).toBe("done"),
    );
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([2]);
  });

  it("ページの取得中に表示中の動画の知らせが届いたら、ページを反映したあとでも取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));
    getVideo.mockResolvedValue({ ...item(2), previewState: "done" });

    act(() => result.current.loadMore());
    await emitServerEvent("video", { id: 2 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(1));

    await act(async () => calls[1]?.resolve(page([2, 3])));

    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        itemVideos(result.current.items)
          .filter((video) => video.id === 2)
          .every((video) => video.previewState === "done"),
      ).toBe(true),
    );
  });

  it("取り直した動画が索引から消えていたら、一覧から外す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2])));
    const { RequestFailed } = await import("./client");
    getVideo.mockRejectedValue(new RequestFailed(404, "not_found", "見つかりません"));

    await emitServerEvent("video", { id: 2 });

    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1]),
    );
    expect(result.current.total).toBe(99);
  });

  it("別のタブで束ねられて代表でなくなった動画は、代表が出ていれば一覧から外す（030 R-9）", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));
    getVideo.mockImplementation((id: number) =>
      Promise.resolve({ ...item(id), versions: { count: 2, representativeId: 1 } }),
    );

    await emitServerEvent("video", { id: 2 });

    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 3]),
    );
    expect(result.current.total).toBe(99);
    expect(calls).toHaveLength(1);
  });

  // Only the server knows whether a new representative matches these conditions; resync rather than insert it.
  it("代表が一覧に出ていない動画が畳まれたら、代表を差し込まずに一覧を読み直す（030 R-9）", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));
    getVideo.mockImplementation((id: number) =>
      Promise.resolve({ ...item(id), versions: { count: 2, representativeId: 9 } }),
    );

    await emitServerEvent("video", { id: 2 });

    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1]?.params.cursor).toBeUndefined();
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([2]);
    expect(itemVideos(result.current.items).map((video) => video.id)).not.toContain(9);
    await act(async () => calls[1]?.resolve({ items: [item(1), item(3)], total: 2 }));

    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 3]),
    );
    expect(result.current.total).toBe(2);
  });

  it("tag の条件を最初の取得にも続きの取得にも渡す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc", tag: [3, 8] }), {
      wrapper: OwnerAudience,
    });

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]?.params.tag).toEqual([3, 8]);

    await act(async () => calls[0]?.resolve(page([1, 2], "cursor-1")));
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1]?.params.tag).toEqual([3, 8]);
  });

  it("付け外しの通知を受けて、表示中の項目の tags を書き換える", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2])));

    act(() => {
      recordAppliedVideoTags(
        [2],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(
      itemVideos(result.current.items).find((video) => video.id === 2)?.tags,
    ).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);
    expect(
      itemVideos(result.current.items).find((video) => video.id === 1)?.tags,
    ).toEqual([]);

    act(() => {
      recordAppliedVideoTags(
        [2],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "remove",
        nextVideoTagsSequence(),
      );
    });
    expect(
      itemVideos(result.current.items).find((video) => video.id === 2)?.tags,
    ).toEqual([]);
  });

  // Retain tag mutations received during page loading and overlay them when their videos arrive.
  it("ページの取得中に届いた付け外しは、その動画を含むページが届いたときに重ねる", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => {
      recordAppliedVideoTags(
        [3],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(itemVideos(result.current.items).some((video) => video.id === 3)).toBe(false);

    await act(async () => calls[1]?.resolve(page([3])));

    await waitFor(() =>
      expect(
        itemVideos(result.current.items).find((video) => video.id === 3)?.tags,
      ).toEqual([
        {
          id: 5,
          name: "旅行",
          manual: true,
          fromFolder: false,
          tentative: false,
          kind: null,
        },
      ]),
    );
  });

  it("取得中に届いた付け外しは、対象のうちそのページに現れた分だけ重ねる", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1], "next")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => {
      recordAppliedVideoTags(
        [1, 3],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(
      itemVideos(result.current.items).find((video) => video.id === 1)?.tags,
    ).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);

    await act(async () => calls[1]?.resolve(page([3])));

    await waitFor(() =>
      expect(
        itemVideos(result.current.items).find((video) => video.id === 3)?.tags,
      ).toEqual([
        {
          id: 5,
          name: "旅行",
          manual: true,
          fromFolder: false,
          tentative: false,
          kind: null,
        },
      ]),
    );
  });

  // Mutated videos may appear several pages later; retain overlays until the matching page arrives.
  it("ページ2の取得中に届いた付け外しは、対象がページ3にあっても、ページ3が届いたときに重ねる", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1], "next-2")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => {
      recordAppliedVideoTags(
        [121],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(itemVideos(result.current.items).some((video) => video.id === 121)).toBe(
      false,
    );

    await act(async () => calls[1]?.resolve(page([2], "next-3")));
    expect(itemVideos(result.current.items).some((video) => video.id === 121)).toBe(
      false,
    );

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(3));
    await act(async () => calls[2]?.resolve(page([121])));

    await waitFor(() =>
      expect(
        itemVideos(result.current.items).find((video) => video.id === 121)?.tags,
      ).toEqual([
        {
          id: 5,
          name: "旅行",
          manual: true,
          fromFolder: false,
          tentative: false,
          kind: null,
        },
      ]),
    );
  });

  it("古い応答が後から届いても、同じ動画・タグの新しい結果を巻き戻さない", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1])));

    const first = nextVideoTagsSequence();
    const second = nextVideoTagsSequence();

    act(() => {
      recordAppliedVideoTags(
        [1],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "remove",
        second,
      );
    });
    expect(
      itemVideos(result.current.items).find((video) => video.id === 1)?.tags,
    ).toEqual([]);

    act(() => {
      recordAppliedVideoTags(
        [1],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        first,
      );
    });
    expect(
      itemVideos(result.current.items).find((video) => video.id === 1)?.tags,
    ).toEqual([]);
  });
});

describe("useVideos の公開の反映", () => {
  function stubVisibility(partial?: number) {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((_, init) => {
        const { videoIds } = JSON.parse(String(init?.body)) as { videoIds: number[] };
        return Promise.resolve(
          new Response(JSON.stringify({ applied: partial ?? new Set(videoIds).size }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }),
    );
  }

  it("切り替えの応答を受けて、表示中の該当の項目の public だけを書き換え、読み直さない", async () => {
    stubVisibility();
    const { updateVideoVisibility } = await import("./visibility");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));

    await act(async () => {
      await updateVideoVisibility([1, 3], true);
    });
    expect(itemVideos(result.current.items).map((video) => video.public)).toEqual([
      true,
      false,
      true,
    ]);
    expect(calls).toHaveLength(1);
    expect(getVideo).not.toHaveBeenCalled();

    await act(async () => {
      await updateVideoVisibility([3], false);
    });
    expect(itemVideos(result.current.items).map((video) => video.public)).toEqual([
      true,
      false,
      false,
    ]);
    vi.unstubAllGlobals();
  });

  it("一部にしか反映されなかった切り替えは、該当の項目を公開にせず取り直す", async () => {
    stubVisibility(1);
    const { updateVideoVisibility } = await import("./visibility");
    getVideo.mockImplementation((id: number) =>
      Promise.resolve({ ...item(id), public: id === 1 }),
    );
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));

    await act(async () => {
      await updateVideoVisibility([1, 3], true);
    });

    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(2));
    expect(getVideo.mock.calls.map(([id]) => id as number).sort()).toEqual([1, 3]);
    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.public)).toEqual([
        true,
        false,
        false,
      ]),
    );
    expect(calls).toHaveLength(1);
    vi.unstubAllGlobals();
  });

  it("取り直しに失敗した動画が残るあいだは一覧の控えを取らせない（PR 338）", async () => {
    stubVisibility(0);
    const { updateVideoVisibility } = await import("./visibility");
    const { clearListSnapshot, saveListSnapshot, takeListSnapshot } =
      await import("./listSnapshot");
    const { RequestFailed } = await import("./client");
    clearListSnapshot();
    let failOnce = true;
    getVideo.mockImplementation((id: number) => {
      if (id === 3 && failOnce) {
        failOnce = false;
        return Promise.reject(new RequestFailed(500, "internal", "失敗"));
      }
      return Promise.resolve({ ...item(id), public: true });
    });
    const { result, unmount } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));

    await act(async () => {
      await updateVideoVisibility([1, 3], true);
    });
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.public)).toEqual([
        true,
        false,
        false,
      ]),
    );

    // Unresolved visibility must block snapshots or returning would restore stale state.
    const snapshot = {
      items: [videoItem(item(3))],
      total: 1,
      hasMore: false,
      scrollY: 0,
    };
    saveListSnapshot({ query: "" }, snapshot);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    await waitFor(() =>
      expect(itemVideos(result.current.items).map((video) => video.public)).toEqual([
        true,
        false,
        true,
      ]),
    );
    await waitFor(() => {
      saveListSnapshot({ query: "" }, snapshot);
      expect(takeListSnapshot({ query: "" })).toBeDefined();
    });
    clearListSnapshot();
    unmount();
    vi.unstubAllGlobals();
  });

  it("一部反映の前に始めた取り直しが後から成功しても、その動画を確かにしない（PR 338）", async () => {
    stubVisibility(0);
    const { updateVideoVisibility } = await import("./visibility");
    const { clearListSnapshot, saveListSnapshot, takeListSnapshot } =
      await import("./listSnapshot");
    const { RequestFailed } = await import("./client");
    clearListSnapshot();
    let answerFirst: ((video: Video) => void) | undefined;
    let attempts = 0;
    getVideo.mockImplementation((id: number) => {
      if (id !== 3) return Promise.resolve(item(id));
      attempts += 1;
      if (attempts === 1) {
        return new Promise<Video>((resolve) => (answerFirst = resolve));
      }
      if (attempts === 2) {
        return Promise.reject(new RequestFailed(500, "internal", "失敗"));
      }
      return Promise.resolve({ ...item(3), public: true });
    });
    const { result, unmount } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));

    await emitServerEvent("video", { id: 3 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledOnce());
    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    await act(async () => answerFirst?.(item(3)));
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(itemVideos(result.current.items)[2]?.public).toBe(false));

    const snapshot = {
      items: [videoItem(item(3))],
      total: 1,
      hasMore: false,
      scrollY: 0,
    };
    saveListSnapshot({ query: "" }, snapshot);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    await emitServerEvent("video", { id: 3 });
    await waitFor(() => expect(itemVideos(result.current.items)[2]?.public).toBe(true));
    await waitFor(() => {
      saveListSnapshot({ query: "" }, snapshot);
      expect(takeListSnapshot({ query: "" })).toBeDefined();
    });
    clearListSnapshot();
    unmount();
    vi.unstubAllGlobals();
  });

  it("一部反映の前に始めたページが後から届いても、その動画を確かにしない（PR 338）", async () => {
    stubVisibility(0);
    const { updateVideoVisibility } = await import("./visibility");
    const { clearListSnapshot, saveListSnapshot, takeListSnapshot } =
      await import("./listSnapshot");
    const { RequestFailed } = await import("./client");
    clearListSnapshot();
    getVideo.mockImplementation((id: number) =>
      id === 3
        ? Promise.reject(new RequestFailed(500, "internal", "失敗"))
        : Promise.resolve(item(id)),
    );
    const { result, unmount } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    await act(async () => calls[1]?.resolve(page([3])));
    await waitFor(() => expect(getVideo).toHaveBeenCalledWith(3, expect.anything()));
    await waitFor(() => expect(itemVideos(result.current.items)).toHaveLength(3));
    expect(itemVideos(result.current.items)[2]?.public).toBe(false);

    const snapshot = {
      items: [videoItem(item(3))],
      total: 1,
      hasMore: false,
      scrollY: 0,
    };
    saveListSnapshot({ query: "" }, snapshot);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    clearListSnapshot();
    unmount();
    vi.unstubAllGlobals();
  });

  it("取り直しに失敗した動画も、全件に反映された切り替えの結果が届けば確かになる（PR 338）", async () => {
    stubVisibility(0);
    const { updateVideoVisibility } = await import("./visibility");
    const { clearListSnapshot, saveListSnapshot, takeListSnapshot } =
      await import("./listSnapshot");
    const { RequestFailed } = await import("./client");
    clearListSnapshot();
    getVideo.mockImplementation((id: number) =>
      id === 3
        ? Promise.reject(new RequestFailed(500, "internal", "失敗"))
        : Promise.resolve(item(id)),
    );
    const { result, unmount } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3])));

    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    await waitFor(() => expect(getVideo).toHaveBeenCalledWith(3, expect.anything()));
    const snapshot = {
      items: [videoItem(item(3))],
      total: 1,
      hasMore: false,
      scrollY: 0,
    };
    saveListSnapshot({ query: "" }, snapshot);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    stubVisibility();
    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    expect(itemVideos(result.current.items)[2]?.public).toBe(true);
    await waitFor(() => {
      saveListSnapshot({ query: "" }, snapshot);
      expect(takeListSnapshot({ query: "" })).toBeDefined();
    });
    expect(getVideo).toHaveBeenCalledTimes(1);
    clearListSnapshot();
    unmount();
    vi.unstubAllGlobals();
  });

  it("ページの取得中に届いた切り替えは、その動画を含むページが届いたときに重ねる", async () => {
    stubVisibility();
    const { updateVideoVisibility } = await import("./visibility");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      await updateVideoVisibility([3], true);
    });
    await act(async () => calls[1]?.resolve(page([3])));

    await waitFor(() =>
      expect(
        itemVideos(result.current.items).find((video) => video.id === 3)?.public,
      ).toBe(true),
    );
    vi.unstubAllGlobals();
  });

  it("ページの取得中に 500 件を超える一括の切り替えがあっても、遅れて届いたページで印を戻さない", async () => {
    stubVisibility();
    const { updateVideoVisibility } = await import("./visibility");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1000], "next")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    const ids = Array.from({ length: 600 }, (_, index) => index + 1);
    await act(async () => {
      await updateVideoVisibility(ids, true);
    });
    await act(async () =>
      calls[1]?.resolve(page(Array.from({ length: 60 }, (_, index) => index + 1))),
    );

    await waitFor(() => expect(itemVideos(result.current.items)).toHaveLength(61));
    expect(
      itemVideos(result.current.items)
        .filter((video) => video.id <= 60)
        .every((video) => video.public),
    ).toBe(true);
    vi.unstubAllGlobals();
  });

  it("切り替えの前に始めた1件の取り直しが後から届いても、公開を巻き戻さない", async () => {
    stubVisibility();
    const { updateVideoVisibility } = await import("./visibility");
    let answer: ((video: Video) => void) | undefined;
    getVideo.mockImplementation(
      () => new Promise<Video>((resolve) => (answer = resolve)),
    );
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([701, 702])));

    await emitServerEvent("video", { id: 701 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledOnce());
    await act(async () => {
      await updateVideoVisibility([701], true);
    });
    expect(itemVideos(result.current.items)[0]?.public).toBe(true);

    await act(async () => answer?.({ ...item(701), title: "新しい題名" }));
    expect(itemVideos(result.current.items)[0]?.public).toBe(true);
    vi.unstubAllGlobals();
  });
});

// Restore group items from a snapshot because this list endpoint returns videos only.
describe("useVideos のグループの項目", () => {
  function group(overrides: Partial<LibraryGroup> = {}): LibraryGroup {
    return {
      id: 7,
      folder: { rootId: 3, path: "A/B" },
      name: "B",
      videoCount: 3,
      watchedCount: 0,
      watchState: "unwatched",
      sizeBytes: 3072,
      addedAt: "2026-09-13T00:00:00Z",
      previews: [],
      openVideoId: 10,
      videoIds: [10, 11, 12],
      tags: [],
      ...overrides,
    };
  }

  function ready(id: number): Video {
    return { ...item(id), previewState: "done" };
  }

  function restoredWithGroup(staleGroups?: number[]) {
    const items: LibraryItem[] = [
      videoItem(ready(1)),
      { kind: "group", group: group() },
      videoItem(ready(2)),
    ];
    return { items, total: 40, cursor: "next", hasMore: true, staleGroups };
  }

  function groups(items: readonly LibraryItem[]): LibraryGroup[] {
    return items.flatMap((entry) => (entry.kind === "group" ? [entry.group] : []));
  }

  function detail(value: LibraryGroup) {
    return { group: value, trail: [], items: [] };
  }

  function deferredGroup() {
    let settle: (value: LibraryGroup) => void = () => undefined;
    let fail: (reason: unknown) => void = () => undefined;
    getGroup.mockImplementationOnce(() =>
      new Promise<LibraryGroup>((resolve, reject) => {
        settle = resolve;
        fail = reject;
      }).then(detail),
    );
    return {
      resolve: (value: LibraryGroup) => settle(value),
      reject: (reason: unknown) => fail(reason),
    };
  }

  it("メンバーの再生位置が保存されたら、そのグループを取り直して差し替え、取り直しの間は変えない", async () => {
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    const before = groups(result.current.items)[0];
    const pending = deferredGroup();

    act(() => {
      recordSavedProgress(
        11,
        { positionMs: 1000, completed: false, updatedAt: "2026-09-23T00:00:00Z" },
        nextProgressSequence(),
      );
    });

    await waitFor(() => expect(getGroup).toHaveBeenCalledTimes(1));
    expect(getGroup.mock.calls[0]?.[0]).toBe(7);
    expect(groups(result.current.items)[0]).toBe(before);

    await act(async () => {
      pending.resolve(group({ watchState: "inProgress", openVideoId: 11 }));
    });
    expect(result.current.items.map((entry) => entry.kind)).toEqual([
      "video",
      "group",
      "video",
    ]);
    expect(groups(result.current.items)[0]).toMatchObject({
      watchState: "inProgress",
      openVideoId: 11,
    });
    expect(listVideos).not.toHaveBeenCalled();
    expect(result.current.cursor).toBe("next");
    expect(result.current.total).toBe(40);
  });

  it("メンバーのタグの付け外しで、そのグループを取り直す", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    const tag = {
      id: 5,
      name: "旅行",
      manual: true,
      fromFolder: false,
      tentative: false,
      kind: null,
    };
    getGroup.mockResolvedValueOnce(detail(group({ tags: [tag] })));

    act(() => {
      recordAppliedVideoTags(
        [12],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });

    await waitFor(() => expect(groups(result.current.items)[0]?.tags).toEqual([tag]));
    expect(getGroup).toHaveBeenCalledTimes(1);
  });

  it("メンバーの video イベントで、そのグループを取り直す", async () => {
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    getGroup.mockResolvedValueOnce(detail(group({ durationMs: 9000 })));

    await emitServerEvent("video", { id: 10 });

    await waitFor(() => expect(groups(result.current.items)[0]?.durationMs).toBe(9000));
    expect(getGroup).toHaveBeenCalledTimes(1);
    expect(getVideo).not.toHaveBeenCalled();
  });

  it("group イベントは、表示中のグループの id だけを取り直す", async () => {
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    getGroup.mockResolvedValueOnce(detail(group({ name: "新しい名前" })));

    await emitServerEvent("group", { ids: [3, 7, 99] });

    await waitFor(() => expect(groups(result.current.items)[0]?.name).toBe("新しい名前"));
    expect(getGroup).toHaveBeenCalledTimes(1);
    expect(getGroup.mock.calls[0]?.[0]).toBe(7);
  });

  it("ライブラリは group イベントで、表示中でないグループでも読み込み済みのページを条件を保ったまま読み直す", async () => {
    listLibrary.mockImplementation((params: ListVideosParams) => listVideos(params));
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc", query: "旅" }, undefined, "library"),
      { wrapper: OwnerAudience },
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => {
      calls[0]?.resolve(page([10, 2]));
    });
    expect(result.current.items.map((entry) => entry.kind)).toEqual(["video", "video"]);

    await emitServerEvent("group", { ids: [7] });
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(getGroup).not.toHaveBeenCalled();
    expect(calls[1]?.params).toMatchObject({ sort: "addedDesc", query: "旅" });
    expect(calls[1]?.params.cursor).toBeUndefined();
    expect(result.current.items).toHaveLength(2);

    await act(async () => {
      calls[1]?.resolve({
        items: [{ kind: "group", group: group({ videoIds: [10] }) }, videoItem(item(2))],
        total: 2,
      } as unknown as VideoPage);
    });
    expect(result.current.items.map((entry) => entry.kind)).toEqual(["group", "video"]);
    expect(groups(result.current.items)[0]?.id).toBe(7);
  });

  it.each([
    ["所有者", OwnerAudience],
    ["ゲスト", undefined],
  ])(
    "絞り込みの無いライブラリは smartFolder イベントで読み込み済みのページを読み直す（%s、docs/design-docs/catalog-organization.md）",
    async (_who, wrapper) => {
      listLibrary.mockImplementation((params: ListVideosParams) => listVideos(params));
      const { result } = renderHook(
        () => useVideos({ sort: "addedDesc", favorite: true }, undefined, "library"),
        { wrapper },
      );
      await waitFor(() => expect(calls).toHaveLength(1));
      await act(async () => {
        calls[0]?.resolve(page([2]));
      });

      await emitServerEvent("smartFolder", { ids: [9] });
      await waitFor(() => expect(calls).toHaveLength(2));
      expect(calls[1]?.params).toMatchObject({ sort: "addedDesc", favorite: true });
      expect(calls[1]?.params.cursor).toBeUndefined();

      await act(async () => {
        calls[1]?.resolve({
          items: [
            { kind: "smartFolder", smartFolder: { id: 9, name: "猫" } },
            videoItem(item(2)),
          ],
          total: 2,
        } as unknown as VideoPage);
      });
      expect(result.current.items.map((entry) => entry.kind)).toEqual([
        "smartFolder",
        "video",
      ]);
    },
  );

  it("フォルダのカードが並ばない一覧は smartFolder イベントで読み直さない", async () => {
    listLibrary.mockImplementation((params: ListVideosParams) => listVideos(params));
    renderHook(
      () => useVideos({ sort: "addedDesc", query: "旅" }, undefined, "library"),
      {
        wrapper: OwnerAudience,
      },
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => {
      calls[0]?.resolve(page([2]));
    });
    renderHook(() => useVideos({ sort: "addedDesc" }), { wrapper: OwnerAudience });
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => {
      calls[1]?.resolve(page([3]));
    });

    await emitServerEvent("smartFolder", { ids: [9] });

    expect(calls).toHaveLength(2);
  });

  it("ゲストにも group イベントで表示中のグループを取り直す", async () => {
    const { result } = renderHook(() =>
      useVideos({ sort: "addedDesc" }, restoredWithGroup()),
    );
    getGroup.mockResolvedValueOnce(detail(group({ name: "新しい名前" })));

    await emitServerEvent("group", { ids: [7] });

    await waitFor(() => expect(groups(result.current.items)[0]?.name).toBe("新しい名前"));
    expect(getGroup).toHaveBeenCalledTimes(1);
  });

  it("ページの取得中に届いた group イベントは、ページを反映したあとでそのページのグループを取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));

    await emitServerEvent("group", { ids: [7] });
    expect(getGroup).not.toHaveBeenCalled();
    getGroup.mockResolvedValueOnce(detail(group({ name: "新しい名前" })));

    await act(async () => {
      calls[0]?.resolve({
        items: [{ kind: "group", group: group() }],
        total: 1,
      } as unknown as VideoPage);
    });

    await waitFor(() => expect(groups(result.current.items)[0]?.name).toBe("新しい名前"));
    expect(getGroup).toHaveBeenCalledTimes(1);
    expect(getGroup.mock.calls[0]?.[0]).toBe(7);
  });

  it("メンバーでない動画の変化では取りに行かない", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    renderHook(() => useVideos({ sort: "addedDesc" }, restoredWithGroup()), {
      wrapper: OwnerAudience,
    });

    act(() => {
      recordSavedProgress(
        1,
        { positionMs: 1000, completed: false, updatedAt: "2026-09-23T00:00:00Z" },
        nextProgressSequence(),
      );
      recordAppliedVideoTags(
        [2],
        { kind: null, id: 5, name: "旅行", tentative: false },
        "add",
        nextVideoTagsSequence(),
      );
    });
    await emitServerEvent("video", { id: 99 });

    expect(getGroup).not.toHaveBeenCalled();
  });

  it("取り直しが 404 なら、その項目を一覧から外し、total は次に読むまで変えない", async () => {
    const { RequestFailed } = await import("./client");
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    getGroup.mockRejectedValueOnce(new RequestFailed(404, "not_found", "見つかりません"));

    await emitServerEvent("video", { id: 11 });

    await waitFor(() => expect(groups(result.current.items)).toHaveLength(0));
    expect(itemVideos(result.current.items).map((video) => video.id)).toEqual([1, 2]);
    expect(result.current.total).toBe(40);
    expect(result.current.error).toBeNull();
  });

  it("取り直しが一時的に失敗したら、項目をそのまま残す", async () => {
    const { RequestFailed } = await import("./client");
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    getGroup.mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"));

    await emitServerEvent("video", { id: 11 });

    await waitFor(() => expect(getGroup).toHaveBeenCalledTimes(1));
    expect(groups(result.current.items)).toEqual([group()]);
  });

  it("控えの後にメンバーが変わったグループは、戻ったときに取り直す", async () => {
    getGroup.mockResolvedValueOnce(detail(group({ watchedCount: 1 })));
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup([7, 99])),
      { wrapper: OwnerAudience },
    );

    await waitFor(() => expect(groups(result.current.items)[0]?.watchedCount).toBe(1));
    expect(getGroup).toHaveBeenCalledTimes(1);
    expect(getGroup.mock.calls[0]?.[0]).toBe(7);
    expect(listVideos).not.toHaveBeenCalled();
  });

  it("取り直しが済むまでは、控えに残す取り直しの印としてグループを返す", async () => {
    const pending = deferredGroup();
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup([7])),
      { wrapper: OwnerAudience },
    );
    await waitFor(() => expect(getGroup).toHaveBeenCalledTimes(1));

    expect(result.current.staleGroups()).toEqual([7]);

    await act(async () => {
      pending.resolve(group({ watchedCount: 1 }));
    });
    expect(result.current.staleGroups()).toEqual([]);
  });

  it("取り直しが一時的に失敗したグループは、控えに残す印として返し続ける", async () => {
    const { RequestFailed } = await import("./client");
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    expect(result.current.staleGroups()).toEqual([]);
    getGroup.mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"));

    await emitServerEvent("video", { id: 11 });
    await waitFor(() => expect(getGroup).toHaveBeenCalledTimes(1));

    await waitFor(() => expect(result.current.staleGroups()).toEqual([7]));
  });

  it("取り直しが 404 で外れたグループは、印として返さない", async () => {
    const { RequestFailed } = await import("./client");
    const { result } = renderHook(
      () => useVideos({ sort: "addedDesc" }, restoredWithGroup()),
      { wrapper: OwnerAudience },
    );
    getGroup.mockRejectedValueOnce(new RequestFailed(404, "not_found", "見つかりません"));

    await emitServerEvent("video", { id: 11 });

    await waitFor(() => expect(groups(result.current.items)).toHaveLength(0));
    expect(result.current.staleGroups()).toEqual([]);
  });

  it("条件を変えて読み直したら、前の一覧のグループの取り直しの応答を重ねない", async () => {
    const { result, rerender } = renderHook(
      ({ sort }: { sort: VideoSort }) => useVideos({ sort }, restoredWithGroup()),
      { wrapper: OwnerAudience, initialProps: { sort: "addedDesc" } },
    );
    const pending = deferredGroup();
    await emitServerEvent("video", { id: 11 });
    await waitFor(() => expect(getGroup).toHaveBeenCalledTimes(1));

    rerender({ sort: "titleAsc" });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([7])));
    await act(async () => {
      pending.resolve(group({ name: "古い応答" }));
    });

    expect(result.current.items).toEqual([videoItem(item(7))]);
  });
});
