import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OwnerAudience } from "../testing/audience";
import { RequestFailed, type Video } from "./client";
import { type FavoritesChange, subscribeFavorites, updateFavorites } from "./favorites";
import {
  currentEventSource,
  emitServerEvent,
  FakeEventSource,
  installFakeEventSource,
} from "./fakeEventSource";

const { getVideo, getRelatedVideos } = vi.hoisted(() => ({
  getVideo: vi.fn(),
  getRelatedVideos: vi.fn(),
}));

vi.mock("./client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./client")>()),
  getVideo,
  getRelatedVideos,
}));

const { detailMark, isProcessing, useRelatedVideos, useVideoDetail } =
  await import("./useVideoDetail");

const done: Video = {
  id: 7,
  title: "動画",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  seekThumbnailState: "done",
  durationMs: 1000,
  videoCodec: "h264",
  tags: [],
};

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("isProcessing", () => {
  it.each([
    [{ probeState: "pending" }, true],
    [{ thumbnailState: "pending" }, true],
    [{ seekThumbnailState: "pending" }, true],
    [{ previewState: "pending" }, true],
    [{ thumbnailState: "failed", previewState: "failed" }, false],
    [{}, false],
    // 読み取りの失敗は終わりとして扱う。プレビューは読み取りの成功後にしか積まれない。
    [{ probeState: "failed", previewState: "pending", thumbnailState: "pending" }, false],
  ] as [Partial<Video>, boolean][])("%j → %s", (overrides, expected) => {
    expect(isProcessing({ ...done, ...overrides })).toBe(expected);
  });
});

describe("useVideoDetail", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    installFakeEventSource();
    getVideo.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("ゲストでは変化の知らせ（/api/events）を購読しない", async () => {
    getVideo.mockResolvedValue(done);
    // AudienceProvider の外の既定はゲストである。
    const { result } = renderHook(() => useVideoDetail(7));
    await flush();
    expect(result.current.state).toMatchObject({ kind: "ready" });
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it("その動画が変わったという知らせでだけ取り直し、一定間隔では問い合わせない", async () => {
    getVideo
      .mockResolvedValueOnce({
        ...done,
        probeState: "pending",
        seekThumbnailState: undefined,
      })
      .mockResolvedValueOnce({ ...done, previewState: "pending" })
      .mockResolvedValueOnce(done);
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    expect(result.current.state).toMatchObject({
      kind: "ready",
      video: { probeState: "pending" },
    });

    await advance(10_000);
    expect(getVideo).toHaveBeenCalledTimes(1);

    // 別の動画の知らせでは取り直さない。
    await emitServerEvent("video", { id: 8 });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
    expect(result.current.state).toMatchObject({ video: { previewState: "pending" } });

    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(result.current.state).toMatchObject({ video: { previewState: "done" } });
  });

  it("replace は応答の動画を手元の 1 件にし、先に始めた取り直しの応答で巻き戻さない", async () => {
    let answer: ((video: Video) => void) | undefined;
    getVideo.mockResolvedValueOnce(done).mockImplementationOnce(
      () =>
        new Promise<Video>((resolve) => {
          answer = resolve;
        }),
    );
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);

    const mark = detailMark();
    act(() =>
      result.current.replace(
        { ...done, title: "新しい名前", displayName: "新しい名前" },
        mark,
      ),
    );
    expect(result.current.state).toMatchObject({ video: { title: "新しい名前" } });

    answer?.(done);
    await flush();
    expect(result.current.state).toMatchObject({ video: { title: "新しい名前" } });

    // 別の動画の応答は捨てる。
    act(() => result.current.replace({ ...done, id: 8, title: "別の動画" }, mark));
    expect(result.current.state).toMatchObject({ id: 7, video: { title: "新しい名前" } });
  });

  it("replace は要求の間に反映した公開の切り替えを巻き戻さず、要求の後に始めた取り直しを残す", async () => {
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
    const { updateVideoVisibility } = await import("./visibility");
    let answer: ((video: Video) => void) | undefined;
    getVideo
      .mockResolvedValueOnce(done)
      .mockImplementationOnce(() => new Promise<Video>((resolve) => (answer = resolve)));
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    expect(result.current.state).toMatchObject({ video: { public: false } });

    // 表示名の保存を送り、その応答の前に公開へ切り替わり、別の変化で取り直しが始まる。
    const mark = detailMark();
    await act(async () => {
      await updateVideoVisibility([7], true);
    });
    await emitServerEvent("video", { id: 7 });
    expect(getVideo).toHaveBeenCalledTimes(2);

    // 表示名の応答は、切り替える前の public: false を読んでいる。
    act(() =>
      result.current.replace(
        { ...done, public: false, title: "新しい名前", displayName: "新しい名前" },
        mark,
      ),
    );
    expect(result.current.state).toMatchObject({
      video: { public: true, title: "新しい名前" },
    });

    // 保存の後に始めた取り直しは打ち切らない。
    await act(async () =>
      answer?.({
        ...done,
        public: true,
        title: "新しい名前",
        displayName: "新しい名前",
        previewState: "pending",
      }),
    );
    expect(result.current.state).toMatchObject({
      video: { public: true, title: "新しい名前", previewState: "pending" },
    });
  });

  it("公開を切り替える前に始めた取り直しが後から届いても、公開の表示を巻き戻さない", async () => {
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
    const { updateVideoVisibility } = await import("./visibility");
    let answer: ((video: Video) => void) | undefined;
    getVideo
      .mockResolvedValueOnce(done)
      .mockImplementationOnce(() => new Promise<Video>((resolve) => (answer = resolve)));
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    expect(result.current.state).toMatchObject({ video: { public: false } });

    // 処理状態の知らせで取り直しが始まり、その応答の前に公開へ切り替わる。
    await emitServerEvent("video", { id: 7 });
    expect(getVideo).toHaveBeenCalledTimes(2);
    await act(async () => {
      await updateVideoVisibility([7], true);
    });
    expect(result.current.state).toMatchObject({ video: { public: true } });

    // 切り替える前に読んだ内容（public: false）が遅れて届く。
    await act(async () => answer?.({ ...done, previewState: "pending" }));
    expect(result.current.state).toMatchObject({
      video: { public: true, previewState: "pending" },
    });
  });

  it("切り替えが反映されなかった（applied が 0）なら公開にせず、取り直す", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(() =>
        Promise.resolve(
          new Response(JSON.stringify({ applied: 0 }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ),
    );
    const { updateVideoVisibility } = await import("./visibility");
    getVideo.mockResolvedValue(done);
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    await act(async () => {
      await updateVideoVisibility([7], true);
    });
    await flush();

    expect(getVideo).toHaveBeenCalledTimes(2);
    expect(result.current.state).toMatchObject({
      kind: "ready",
      video: { public: false },
    });
  });

  it("知らせの接続をつなぎ直したら、切れていた間の変化を取り戻す", async () => {
    getVideo
      .mockResolvedValueOnce({ ...done, thumbnailState: "pending" })
      .mockResolvedValueOnce(done);
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    await emitServerEvent("open");
    await flush();

    expect(getVideo).toHaveBeenCalledTimes(2);
    expect(result.current.state).toMatchObject({ video: { thumbnailState: "done" } });
  });

  it("取り直しが 404 なら missing にする", async () => {
    getVideo
      .mockResolvedValueOnce({ ...done, thumbnailState: "pending" })
      .mockRejectedValueOnce(new RequestFailed(404, "not_found", "見つかりません"));
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(result.current.state).toEqual({ kind: "missing", id: 7 });
  });

  it("取り直しの一時的な失敗では控えを残す", async () => {
    getVideo
      .mockResolvedValueOnce({ ...done, thumbnailState: "pending" })
      .mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"))
      .mockResolvedValueOnce(done);
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(result.current.state).toMatchObject({
      kind: "ready",
      video: { thumbnailState: "pending" },
    });
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(result.current.state).toMatchObject({ video: { thumbnailState: "done" } });
  });

  it("最初の取得の失敗は理由を持つ", async () => {
    getVideo.mockRejectedValue(new RequestFailed(500, "internal", "壊れています"));
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    expect(result.current.state).toEqual({
      kind: "failed",
      id: 7,
      reason: "Something went wrong on the server.",
    });
  });

  it("id の形が正しくなければ要求せずに missing にする", async () => {
    const { result } = renderHook(() => useVideoDetail(Number.NaN), {
      wrapper: OwnerAudience,
    });
    await flush();
    expect(result.current.state.kind).toBe("missing");
    expect(getVideo).not.toHaveBeenCalled();
  });

  it("画面を離れると要求を打ち切り、知らせを受けても取り直さない", async () => {
    let signal: AbortSignal | undefined;
    getVideo.mockResolvedValueOnce({ ...done, thumbnailState: "pending" });
    getVideo.mockImplementationOnce((_id: number, next: AbortSignal) => {
      signal = next;
      return new Promise(() => undefined);
    });
    const { unmount } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    await emitServerEvent("video", { id: 7 });
    expect(signal?.aborted).toBe(false);
    const source = currentEventSource();
    unmount();
    expect(signal?.aborted).toBe(true);
    await act(async () => source.dispatch("video", { id: 7 }));
    expect(getVideo).toHaveBeenCalledTimes(2);
  });

  it("別の動画へ移ると前の動画の知らせでは取り直さない", async () => {
    getVideo.mockImplementation((id: number) =>
      Promise.resolve({ ...done, id, thumbnailState: id === 7 ? "pending" : "done" }),
    );
    const { result, rerender } = renderHook(({ id }) => useVideoDetail(id), {
      initialProps: { id: 7 },
      wrapper: OwnerAudience,
    });
    await flush();
    rerender({ id: 8 });
    await flush();
    expect(result.current.state).toMatchObject({ kind: "ready", id: 8 });
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(getVideo.mock.calls.map(([id]) => id as number)).toEqual([7, 8]);
  });

  it("refresh はすぐ取り直し、取得が終わると解決する", async () => {
    getVideo
      .mockResolvedValueOnce({ ...done, probeState: "failed" })
      .mockResolvedValueOnce({ ...done, probeState: "pending" });
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    let resolved = false;
    await act(async () => {
      await result.current.refresh().then(() => {
        resolved = true;
      });
    });
    expect(resolved).toBe(true);
    expect(result.current.state).toMatchObject({ video: { probeState: "pending" } });
    expect(getVideo).toHaveBeenCalledTimes(2);
  });

  it("代表以外のバージョンの付け外しの後、取り直した値を一覧の代表へ知らせる", async () => {
    const version: Video = {
      ...done,
      id: 12,
      favorite: false,
      versions: { count: 2, representativeId: 11 },
    };
    getVideo
      .mockResolvedValueOnce(version)
      .mockResolvedValueOnce({ ...version, favorite: true });
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValue(
        new Response(JSON.stringify({ appliedVideos: 1, appliedFolders: 0 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    const changes: FavoritesChange[] = [];
    const unsubscribe = subscribeFavorites((change) => changes.push(change));
    const { result } = renderHook(() => useVideoDetail(12), { wrapper: OwnerAudience });
    await flush();

    await act(async () => {
      await result.current.setFavorite(true);
    });

    expect(result.current.state).toMatchObject({ video: { id: 12, favorite: true } });
    expect(changes).toContainEqual({ videoIds: [11], folders: [], favorite: true });
    unsubscribe();
  });

  it("付け外しの後の取り直しが失敗したら、古い値を代表へ知らせない", async () => {
    const version: Video = {
      ...done,
      id: 12,
      favorite: false,
      versions: { count: 2, representativeId: 11 },
    };
    getVideo
      .mockResolvedValueOnce(version)
      .mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"));
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockImplementation(() =>
        Promise.resolve(
          new Response(JSON.stringify({ appliedVideos: 1, appliedFolders: 0 }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ),
    );
    // 前に別の動画の付け外しが決着していて、最初の取得はその後に読んだ。
    await updateFavorites([99], [], true);
    const changes: FavoritesChange[] = [];
    const unsubscribe = subscribeFavorites((change) => changes.push(change));
    const { result } = renderHook(() => useVideoDetail(12), { wrapper: OwnerAudience });
    await flush();

    await act(async () => {
      await result.current.setFavorite(true);
    });

    expect(result.current.state).toMatchObject({ video: { id: 12, favorite: false } });
    expect(changes).toEqual([{ videoIds: [12], folders: [], favorite: true }]);
    unsubscribe();
  });

  it("代表の付け外しでは、代表への知らせを重ねない", async () => {
    const representative: Video = {
      ...done,
      id: 11,
      favorite: false,
      versions: { count: 2, representativeId: 11 },
    };
    getVideo
      .mockResolvedValueOnce(representative)
      .mockResolvedValueOnce({ ...representative, favorite: true });
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValue(
        new Response(JSON.stringify({ appliedVideos: 1, appliedFolders: 0 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    const changes: FavoritesChange[] = [];
    const unsubscribe = subscribeFavorites((change) => changes.push(change));
    const { result } = renderHook(() => useVideoDetail(11), { wrapper: OwnerAudience });
    await flush();

    await act(async () => {
      await result.current.setFavorite(true);
    });

    expect(changes).toEqual([{ videoIds: [11], folders: [], favorite: true }]);
    unsubscribe();
  });
});

describe("useRelatedVideos", () => {
  beforeEach(() => getRelatedVideos.mockReset());

  it("rename は保存した表示名をグループのメンバーの並びにある同じ動画へ写す", async () => {
    const folder = { rootId: 1, path: "series" };
    const other: Video = { ...done, id: 8, title: "ep02" };
    getRelatedVideos.mockResolvedValue({
      items: [],
      group: { folder, name: "series", items: [done, other] },
    });
    const { result } = renderHook(() => useRelatedVideos(7), { wrapper: OwnerAudience });
    await act(async () => undefined);

    act(() =>
      result.current.rename({
        ...done,
        title: "夏の旅行",
        fileTitle: "動画",
        displayName: "夏の旅行",
      }),
    );
    const named =
      result.current.state.kind === "ready"
        ? result.current.state.related.group?.items
        : undefined;
    expect(named?.map((item) => item.title)).toEqual(["夏の旅行", "ep02"]);
    expect(named?.[0]).toMatchObject({ fileTitle: "動画", displayName: "夏の旅行" });

    // 解除では displayName を消す。別の動画の応答は写さない。
    act(() => result.current.rename({ ...done, fileTitle: "動画" }));
    act(() => result.current.rename({ ...other, title: "別の動画" }));
    const cleared =
      result.current.state.kind === "ready"
        ? result.current.state.related.group?.items
        : undefined;
    expect(cleared?.map((item) => item.title)).toEqual(["動画", "ep02"]);
    expect(cleared?.[0]?.displayName).toBeUndefined();
  });

  it("rethumb は指定したサムネイルをグループのメンバーの並びにある同じ動画へ写す", async () => {
    const folder = { rootId: 1, path: "series" };
    const other: Video = { ...done, id: 8, title: "ep02" };
    getRelatedVideos.mockResolvedValue({
      items: [],
      group: { folder, name: "series", items: [done, other] },
    });
    const { result } = renderHook(() => useRelatedVideos(7), { wrapper: OwnerAudience });
    await act(async () => undefined);

    act(() =>
      result.current.rethumb({
        ...done,
        thumbnailUrl: "/api/videos/7/thumbnail?v=abc-r2",
        thumbnailPositionMs: 1500,
      }),
    );
    const items = () =>
      result.current.state.kind === "ready"
        ? result.current.state.related.group?.items
        : undefined;
    expect(items()?.[0]).toMatchObject({
      thumbnailUrl: "/api/videos/7/thumbnail?v=abc-r2",
      thumbnailPositionMs: 1500,
    });
    expect(items()?.[1]).toBe(other);

    // 解除では位置を消し、応答の URL にそろえる。別の動画の応答は写さない。
    act(() =>
      result.current.rethumb({ ...done, thumbnailUrl: "/api/videos/7/thumbnail?v=abc" }),
    );
    act(() => result.current.rethumb({ ...other, thumbnailUrl: "/x" }));
    expect(items()?.[0]?.thumbnailUrl).toBe("/api/videos/7/thumbnail?v=abc");
    expect(items()?.[0]?.thumbnailPositionMs).toBeUndefined();
    expect(items()?.[1]).toBe(other);
  });

  it("失敗したら retry で取り直す", async () => {
    getRelatedVideos
      .mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"))
      .mockResolvedValueOnce({ items: [done], nextId: 7 });
    const { result } = renderHook(() => useRelatedVideos(3), { wrapper: OwnerAudience });
    await act(async () => undefined);
    expect(result.current.state.kind).toBe("failed");
    act(() => result.current.retry());
    await act(async () => undefined);
    expect(result.current.state).toMatchObject({
      kind: "ready",
      related: { nextId: 7 },
    });
  });
});
