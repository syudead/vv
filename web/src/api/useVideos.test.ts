import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OwnerAudience } from "../testing/audience";
import type { ListVideosParams, Video, VideoPage, VideoSort } from "./client";
import {
  emitServerEvent,
  FakeEventSource,
  installFakeEventSource,
} from "./fakeEventSource";
import type { VideosCriteria } from "./useVideos";

/**
 * 一覧の読み込みの振る舞いを固定する。
 *
 * カーソルの引き継ぎと打ち切りは目で見て分かる形に現れない。画面を書き換えても
 * **このテストが同じ内容で通ること**が、既存の振る舞いを保った証拠になる。
 */

const { listVideos, listFolderVideos, getVideo } = vi.hoisted(() => ({
  listVideos: vi.fn(),
  listFolderVideos: vi.fn(),
  getVideo: vi.fn(),
}));

vi.mock("./client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./client")>()),
  listVideos,
  listFolderVideos,
  getVideo,
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
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

/** page は 1 ページ分の応答を作る。 */
function page(ids: number[], nextCursor?: string): VideoPage {
  return { items: ids.map(item), total: 100, nextCursor };
}

beforeEach(() => {
  calls = [];
  installFakeEventSource();
  getVideo.mockReset();
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
  it("ゲストでは変化の知らせ（/api/events）を購読しない", async () => {
    // AudienceProvider の外の既定はゲストである。
    renderHook(() => useVideos({ sort: "addedDesc" }));
    await waitFor(() => {
      expect(calls).toHaveLength(1);
    });
    expect(FakeEventSource.instances).toHaveLength(0);
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
    expect(result.current.items.map((video) => video.id)).toEqual([2, 3]);
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
    expect(result.current.items.map((video) => video.id)).toEqual([1, 2]);
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

    expect(result.current.items).toEqual([]);
    expect(result.current.error).toContain("再試行してください");
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

    expect(result.current.items.map((video) => video.id)).toEqual([1, 2, 3]);
    expect(result.current.items[1]?.progress).toEqual(progress);
  });

  it("先頭ページの再試行中は前のエラーを消す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.reject(new Error("一時的な失敗")));
    expect(result.current.error).toBe("一時的な失敗");

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

    expect(result.current.items.find((video) => video.id === 2)?.progress).toEqual(
      progress,
    );
    expect(
      result.current.items.find((video) => video.id === 1)?.progress,
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
      ({ query }: { query: string }) => useVideos({ sort: "addedDesc", query }),
      { initialProps: { query: "ね" }, wrapper: OwnerAudience },
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

describe("useVideos（条件と重複）", () => {
  it("条件（検索語・視聴状態・再生可否・並び順・seed）をそのまま要求へ渡す", async () => {
    renderHook(() =>
      useVideos({
        query: "京都",
        watch: "unwatched",
        playable: true,
        sort: "random",
        seed: 42,
      }),
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]?.params).toMatchObject({
      query: "京都",
      watch: "unwatched",
      playable: true,
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
    ["視聴状態", { sort: "addedDesc", watch: "watched" }],
    ["再生可否", { sort: "addedDesc", playable: true }],
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
      // 打ち切られた要求の応答が後から届いても一覧を上書きしない。
      await act(async () => calls[0]?.resolve(page([1, 2, 3])));
      expect(result.current.items.map((video) => video.id)).toEqual([9]);
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

  it("続きのページに既に出た id が含まれていても、一覧に2度出さない", async () => {
    const { result } = renderHook(() => useVideos({ sort: "sizeDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2, 3], "cursor-1")));

    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () => calls[1]?.resolve(page([3, 4, 2, 5, 5])));

    expect(result.current.items.map((video) => video.id)).toEqual([1, 2, 3, 4, 5]);
  });
});

describe("useVideos の準備の反映", () => {
  it("動画が変わった知らせで、その項目だけを取り直して置き換える", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));
    expect(result.current.items).toHaveLength(2);

    getVideo.mockResolvedValue({
      ...item(2),
      title: "代表の所在の題名",
      previewState: "done",
      durationMs: 5000,
    });
    await emitServerEvent("video", { id: 2 });
    await waitFor(() => expect(result.current.items[1]?.previewState).toBe("done"));

    expect(getVideo).toHaveBeenCalledTimes(1);
    expect(getVideo.mock.calls[0]?.[0]).toBe(2);
    // 一覧は読み直さず、読み込んだページと続きの位置を保つ。
    expect(listVideos).toHaveBeenCalledTimes(1);
    expect(result.current.cursor).toBe("next");
    expect(result.current.items[1]).toMatchObject({
      durationMs: 5000,
      // 題名は一覧側の値を残す（フォルダ画面はそのフォルダの所在の題名を出す）。
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

    // 前の一覧で動画 2 の取り直しを始め、応答はまだ返らない。
    let resolveOld: (video: Video) => void = () => {};
    getVideo.mockImplementationOnce(
      () =>
        new Promise<Video>((resolve) => {
          resolveOld = resolve;
        }),
    );
    await emitServerEvent("video", { id: 2 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(1));

    // 条件を変えると、新しいページには準備の済んだ動画 2 が入っている。
    rerender({ criteria: { sort: "addedDesc", query: "新" } });
    await waitFor(() => expect(calls).toHaveLength(2));
    await act(async () =>
      calls[1]?.resolve({ items: [{ ...item(2), previewState: "done" }], total: 1 }),
    );
    expect(result.current.items[0]?.previewState).toBe("done");

    // 遅れて届いた古い取り直し（準備中）は新しい一覧に重ねない。
    await act(async () => resolveOld({ ...item(2), previewState: "pending" }));
    expect(result.current.items[0]?.previewState).toBe("done");
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
      expect(result.current.items.map((video) => video.id)).toEqual([5]),
    );

    const { RequestFailed } = await import("./client");
    await act(async () =>
      rejectOld(new RequestFailed(404, "not_found", "見つかりません")),
    );
    expect(result.current.notFound).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.items.map((video) => video.id)).toEqual([5]);
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

    await waitFor(() => expect(result.current.items[1]?.previewState).toBe("done"));
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
      expect(result.current.items.map((video) => video.id)).toEqual([1]),
    );
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([1, 2]);
  });

  it("ページの取得中に届いた知らせは、ページを反映したあとで取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    getVideo.mockResolvedValue({ ...item(2), previewState: "done" });

    // 一覧の応答はまだ返っていない。その間に動画 2 の準備が終わる。
    await emitServerEvent("video", { id: 2 });
    expect(getVideo).not.toHaveBeenCalled();

    // 応答は知らせより古い内容（準備中）を持っている。
    await act(async () => calls[0]?.resolve(page([1, 2])));

    await waitFor(() => expect(result.current.items[1]?.previewState).toBe("done"));
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([2]);
  });

  it("ページの取得中に表示中の動画の知らせが届いたら、ページを反映したあとでも取り直す", async () => {
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));
    getVideo.mockResolvedValue({ ...item(2), previewState: "done" });

    // 続きの応答がまだ返らない間に、表示中の動画 2 の準備が終わる。
    act(() => result.current.loadMore());
    await emitServerEvent("video", { id: 2 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(1));

    // 並べ替えの値が変わった動画は続きのページにも現れる。遅れて届いた応答は
    // 知らせより古い内容（準備中）を持っている。
    await act(async () => calls[1]?.resolve(page([2, 3])));

    await waitFor(() => expect(getVideo).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        result.current.items
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
      expect(result.current.items.map((video) => video.id)).toEqual([1]),
    );
    expect(result.current.total).toBe(99);
  });

  // issue 267: useVideos は tag の条件を listVideos へ渡し、続きのページの取得でも
  // 同じ条件を渡し続ける。
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

  // issue 267: 付け外しの結果は、表示中の項目の tags へその場で反映される
  // （一覧を取り直さない。Plan の Structural Decisions 7）。
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
        { id: 5, name: "旅行" },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(result.current.items.find((video) => video.id === 2)?.tags).toEqual([
      { id: 5, name: "旅行" },
    ]);
    expect(result.current.items.find((video) => video.id === 1)?.tags).toEqual([]);

    act(() => {
      recordAppliedVideoTags(
        [2],
        { id: 5, name: "旅行" },
        "remove",
        nextVideoTagsSequence(),
      );
    });
    expect(result.current.items.find((video) => video.id === 2)?.tags).toEqual([]);
  });

  // Devin の指摘3: ページの取得中に届いた付け外しは、まだ読み込んでいない
  // 動画には反映しようが無く、そのままだと後から届くページの古い（付け外し
  // 前の）内容で上書きされて消えてしまう。取得中に届いた変更を覚えておき、
  // その後に届いたページにその動画があれば重ねる（changedWhileLoading と
  // 同じ仕組み）。
  it("ページの取得中に届いた付け外しは、その動画を含むページが届いたときに重ねる", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    await act(async () => calls[0]?.resolve(page([1, 2], "next")));

    // 続きのページ（動画3を含む）の取得中に、その動画3へタグが付く。
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => {
      recordAppliedVideoTags(
        [3],
        { id: 5, name: "旅行" },
        "add",
        nextVideoTagsSequence(),
      );
    });
    // まだ一覧に無いので、この時点では何も変わらない。
    expect(result.current.items.some((video) => video.id === 3)).toBe(false);

    // 続きの応答は、その付け外しより古い（タグの付いていない）内容を持つ。
    await act(async () => calls[1]?.resolve(page([3])));

    await waitFor(() =>
      expect(result.current.items.find((video) => video.id === 3)?.tags).toEqual([
        { id: 5, name: "旅行" },
      ]),
    );
  });

  // 動画1・2つ以上へまとめて付けたときも、取得中の動画の分だけを重ねる。
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
        { id: 5, name: "旅行" },
        "add",
        nextVideoTagsSequence(),
      );
    });
    // 動画1は表示中なので、その場で反映される（従来どおり）。
    expect(result.current.items.find((video) => video.id === 1)?.tags).toEqual([
      { id: 5, name: "旅行" },
    ]);

    await act(async () => calls[1]?.resolve(page([3])));

    await waitFor(() =>
      expect(result.current.items.find((video) => video.id === 3)?.tags).toEqual([
        { id: 5, name: "旅行" },
      ]),
    );
  });

  // Devin の指摘3（2回目）: ページ2の取得中に届いた付け外しの対象が、
  // ページ2ではなくさらに次のページ3に現れることがある。ページ2の到着で
  // tagsChangedWhileLoading を空にしてしまうと、続くページ3の取得が始まる
  // 前に消えてしまい、ページ3が届いても重ねられない。続きの取得
  // （loadMore）をまたいで、実際にその動画を含むページが届くまで持ち越す。
  it("ページ2の取得中に届いた付け外しは、対象がページ3にあっても、ページ3が届いたときに重ねる", async () => {
    const { nextVideoTagsSequence, recordAppliedVideoTags } =
      await import("./videoTagsEvents");
    const { result } = renderHook(() => useVideos({ sort: "addedDesc" }), {
      wrapper: OwnerAudience,
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    // ページ1: 動画1つだけ。続きがある。
    await act(async () => calls[0]?.resolve(page([1], "next-2")));

    // ページ2の取得中に、まだどのページにも現れていない動画121へタグを付ける。
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(2));
    act(() => {
      recordAppliedVideoTags(
        [121],
        { id: 5, name: "旅行" },
        "add",
        nextVideoTagsSequence(),
      );
    });
    expect(result.current.items.some((video) => video.id === 121)).toBe(false);

    // ページ2が届く（動画121を含まない）。
    await act(async () => calls[1]?.resolve(page([2], "next-3")));
    expect(result.current.items.some((video) => video.id === 121)).toBe(false);

    // ページ3の取得が始まり、動画121を含む（付け外し前の、古いタグの
    // ままの）内容で届く。
    act(() => result.current.loadMore());
    await waitFor(() => expect(calls).toHaveLength(3));
    await act(async () => calls[2]?.resolve(page([121])));

    await waitFor(() =>
      expect(result.current.items.find((video) => video.id === 121)?.tags).toEqual([
        { id: 5, name: "旅行" },
      ]),
    );
  });

  // N1: 応答が送った順と違う順で届いても、後から送った操作を古い応答で
  // 巻き戻さない。
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

    // 2番目に送った「外す」の応答が先に届く。
    act(() => {
      recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "remove", second);
    });
    expect(result.current.items.find((video) => video.id === 1)?.tags).toEqual([]);

    // 1番目に送った「付ける」の応答が遅れて届いても、2番目の結果を上書きしない。
    act(() => {
      recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "add", first);
    });
    expect(result.current.items.find((video) => video.id === 1)?.tags).toEqual([]);
  });
});

// 公開・非公開の切り替えの後の一覧（issue 305）。読み直さず、該当の項目の public を差し替える。
describe("useVideos の公開の反映", () => {
  function stubVisibility() {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(() =>
        Promise.resolve(
          new Response(JSON.stringify({ applied: 1 }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ),
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
    expect(result.current.items.map((video) => video.public)).toEqual([
      true,
      false,
      true,
    ]);
    expect(calls).toHaveLength(1);
    expect(getVideo).not.toHaveBeenCalled();

    await act(async () => {
      await updateVideoVisibility([3], false);
    });
    expect(result.current.items.map((video) => video.public)).toEqual([
      true,
      false,
      false,
    ]);
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
      expect(result.current.items.find((video) => video.id === 3)?.public).toBe(true),
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
    // 続きの 60 件を取りに行っている間に、ID 1〜600 を公開にする。
    const ids = Array.from({ length: 600 }, (_, index) => index + 1);
    await act(async () => {
      await updateVideoVisibility(ids, true);
    });
    // 切り替える前の内容（public: false）のページが後から届く。
    await act(async () =>
      calls[1]?.resolve(page(Array.from({ length: 60 }, (_, index) => index + 1))),
    );

    await waitFor(() => expect(result.current.items).toHaveLength(61));
    expect(
      result.current.items
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

    // 動画 701 が変わった知らせで取り直しが始まる。
    await emitServerEvent("video", { id: 701 });
    await waitFor(() => expect(getVideo).toHaveBeenCalledOnce());
    await act(async () => {
      await updateVideoVisibility([701], true);
    });
    expect(result.current.items[0]?.public).toBe(true);

    // 切り替える前に読んだ内容が遅れて届く。
    await act(async () => answer?.({ ...item(701), title: "新しい題名" }));
    expect(result.current.items[0]?.public).toBe(true);
    vi.unstubAllGlobals();
  });
});
