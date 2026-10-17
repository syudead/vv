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

function workInfoCoded(code: string): NonNullable<Video["workInfo"]> {
  return { code, dates: [], people: [], organizations: [], urls: [], externalIds: [] };
}

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

  it("group を取得に渡し、同じ動画で group だけが変わったら読み込み中に戻さずに取り直す", async () => {
    getVideo.mockResolvedValue(done);
    const { result, rerender } = renderHook(
      ({ group }: { group?: number }) => useVideoDetail(7, group),
      { initialProps: { group: 3 } as { group?: number } },
    );
    await flush();
    expect(getVideo).toHaveBeenLastCalledWith(7, expect.anything(), 3, undefined);
    rerender({ group: undefined });
    expect(result.current.state).toMatchObject({ kind: "ready" });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
    expect(getVideo).toHaveBeenLastCalledWith(7, expect.anything(), undefined, undefined);
  });

  it("group があるとき replace は応答のグループではなく手元のグループを残す", async () => {
    const picks = { id: 9, name: "picks", position: 2, count: 3 };
    const series = { id: 1, name: "series", position: 1, count: 5 };
    getVideo.mockResolvedValue({ ...done, group: picks });
    const { result, rerender } = renderHook(
      ({ group }: { group?: number }) => useVideoDetail(7, group),
      { initialProps: { group: 9 } as { group?: number } },
    );
    await flush();

    // 変更の要求は group を受け取らず、応答は id のいちばん小さいグループを返す。
    act(() =>
      result.current.replace(
        { ...done, title: "新しい名前", group: series },
        detailMark(),
        "keep",
      ),
    );
    expect(result.current.state).toMatchObject({
      video: { title: "新しい名前", group: picks },
    });

    // group が無ければ応答のまま。
    getVideo.mockResolvedValue({ ...done, group: series });
    rerender({ group: undefined });
    await flush();
    act(() => result.current.replace({ ...done, group: series }, detailMark(), "keep"));
    expect(result.current.state).toMatchObject({ video: { group: series } });
  });

  it("ゲストは変化の知らせ（/api/events）のうち group だけで取り直す", async () => {
    getVideo.mockResolvedValue({
      ...done,
      group: { id: 9, name: "picks", position: 1, count: 3 },
    });
    // AudienceProvider の外の既定はゲストである。
    const { result } = renderHook(() => useVideoDetail(7));
    await flush();
    expect(result.current.state).toMatchObject({ kind: "ready" });
    expect(FakeEventSource.instances).toHaveLength(1);

    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    await emitServerEvent("group", { ids: [9] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
  });

  it("グループの行か開いたグループが変わった group の知らせで、読み込み中に戻さずに取り直す", async () => {
    const picks = { id: 9, name: "picks", position: 1, count: 3 };
    getVideo
      .mockResolvedValueOnce({ ...done, group: picks })
      .mockResolvedValueOnce({ ...done, group: { ...picks, position: 2 } })
      .mockResolvedValue({ ...done, group: { ...picks, position: 2 } });
    const { result } = renderHook(() => useVideoDetail(7, 4), {
      wrapper: OwnerAudience,
    });
    await flush();

    // 関わらないグループの知らせでは取り直さない。開いたグループ 4 は動画を持たず、表示中の
    // グループ 9 はフォールバックなので、9 より id の大きいグループは文脈を替えない。
    await emitServerEvent("group", { ids: [12, 15] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    // 別の画面で並べ替えると、変わったグループ（祖先を含む）の id が届く。
    await emitServerEvent("group", { ids: [9, 2] });
    expect(result.current.state).toMatchObject({ kind: "ready" });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
    expect(getVideo).toHaveBeenLastCalledWith(7, expect.anything(), 4, undefined);
    expect(result.current.state).toMatchObject({ video: { group: { position: 2 } } });

    // 開いたグループ（?group）の知らせでも取り直す。
    await emitServerEvent("group", { ids: [4] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(3);
  });

  describe("開いたスマートフォルダ", () => {
    const inFolder = { id: 5, name: "タグ: 猫", position: 2, count: 4 };

    it("smartFolder を取得に渡し、同じ動画で smartFolder だけが変わったら取り直す", async () => {
      getVideo.mockResolvedValue({ ...done, smartFolder: inFolder });
      const { result, rerender } = renderHook(
        ({ folder }: { folder?: number }) =>
          useVideoDetail(7, undefined, undefined, folder),
        { initialProps: { folder: 5 } as { folder?: number } },
      );
      await flush();
      expect(getVideo).toHaveBeenLastCalledWith(7, expect.anything(), undefined, 5);
      rerender({ folder: undefined });
      expect(result.current.state).toMatchObject({ kind: "ready" });
      await flush();
      expect(getVideo).toHaveBeenLastCalledWith(
        7,
        expect.anything(),
        undefined,
        undefined,
      );
    });

    it("smartFolder があるとき replace は手元のフォルダの行を残す", async () => {
      getVideo.mockResolvedValue({ ...done, smartFolder: inFolder });
      const { result } = renderHook(() => useVideoDetail(7, undefined, undefined, 5));
      await flush();
      // 変更の要求は smartFolder を受け取らず、応答にフォルダは無い。
      act(() =>
        result.current.replace({ ...done, title: "新しい名前" }, detailMark(), "keep"),
      );
      expect(result.current.state).toMatchObject({
        video: { title: "新しい名前", smartFolder: inFolder },
      });
      expect(result.current.state).not.toHaveProperty("video.group");
    });

    it("最初の取得の前の replace は応答のまま置く", async () => {
      getVideo.mockReturnValue(new Promise(() => undefined));
      const { result } = renderHook(() => useVideoDetail(7, undefined, undefined, 5));
      act(() =>
        result.current.replace({ ...done, title: "先に保存" }, detailMark(), "keep"),
      );
      expect(result.current.state).toMatchObject({ video: { title: "先に保存" } });
      expect(result.current.state).not.toHaveProperty("video.smartFolder");
    });

    it("開いたフォルダを名指した smartFolder の知らせとどの group の知らせでも取り直し、ほかのフォルダでは取り直さない", async () => {
      getVideo.mockResolvedValue({ ...done, smartFolder: inFolder });
      renderHook(() => useVideoDetail(7, undefined, undefined, 5), {
        wrapper: OwnerAudience,
      });
      await flush();
      await emitServerEvent("smartFolder", { ids: [6] });
      await flush();
      expect(getVideo).toHaveBeenCalledTimes(1);
      await emitServerEvent("smartFolder", { ids: [6, 5] });
      await flush();
      expect(getVideo).toHaveBeenCalledTimes(2);
      await emitServerEvent("group", { ids: [12] });
      await flush();
      expect(getVideo).toHaveBeenCalledTimes(3);
    });

    it("ゲストも開いたフォルダの知らせで取り直す", async () => {
      getVideo.mockResolvedValue({ ...done, smartFolder: inFolder });
      renderHook(() => useVideoDetail(7, undefined, undefined, 5));
      await flush();
      await emitServerEvent("smartFolder", { ids: [5] });
      await flush();
      expect(getVideo).toHaveBeenCalledTimes(2);
    });

    it("フォルダの文脈が無ければ smartFolder の知らせでは取り直さない", async () => {
      getVideo.mockResolvedValue(done);
      renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
      await flush();
      await emitServerEvent("smartFolder", { ids: [5] });
      await flush();
      expect(getVideo).toHaveBeenCalledTimes(1);
    });
  });

  it("フォールバックのグループを表示中なら、それより id の小さいグループの知らせでも取り直す", async () => {
    const picks = { id: 9, name: "picks", position: 1, count: 3 };
    getVideo.mockResolvedValue({ ...done, group: picks });
    renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    await emitServerEvent("group", { ids: [12] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    // グループ 3 に動画が足されると、文脈は id のいちばん小さいグループ 3 に替わる。
    await emitServerEvent("group", { ids: [3] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
  });

  it("開いたグループを表示中なら、他のグループの知らせでは取り直さない", async () => {
    const picks = { id: 9, name: "picks", position: 1, count: 3 };
    getVideo.mockResolvedValue({ ...done, group: picks });
    renderHook(() => useVideoDetail(7, 9), { wrapper: OwnerAudience });
    await flush();

    await emitServerEvent("group", { ids: [3, 12] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    await emitServerEvent("group", { ids: [9] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
  });

  it("グループに入っていない動画は、どの group の知らせでも取り直す（足されたかもしれない）", async () => {
    getVideo.mockResolvedValue(done);
    renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();
    await emitServerEvent("group", { ids: [12] });
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
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
        "keep",
      ),
    );
    expect(result.current.state).toMatchObject({ video: { title: "新しい名前" } });

    answer?.(done);
    await flush();
    expect(result.current.state).toMatchObject({ video: { title: "新しい名前" } });

    // 別の動画の応答は捨てる。
    act(() =>
      result.current.replace({ ...done, id: 8, title: "別の動画" }, mark, "keep"),
    );
    expect(result.current.state).toMatchObject({ id: 7, video: { title: "新しい名前" } });
  });

  it("replace の keep は、応答に無い作品情報に手元の値を残す", async () => {
    getVideo.mockResolvedValue({ ...done, workInfo: workInfoCoded("AB-123") });
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    act(() =>
      result.current.replace(
        { ...done, title: "新しい名前", displayName: "新しい名前" },
        detailMark(),
        "keep",
      ),
    );
    expect(result.current.state).toMatchObject({
      video: { title: "新しい名前", workInfo: { code: "AB-123" } },
    });
    expect(getVideo).toHaveBeenCalledTimes(1);
  });

  it("replace は、応答に無い追加の情報に手元の値を残す", async () => {
    const mediaInfo: NonNullable<Video["mediaInfo"]> = {
      audioTracks: [{ index: 1, codec: "aac", language: "jpn" }],
    };
    getVideo.mockResolvedValue({ ...done, mediaInfo });
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    act(() =>
      result.current.replace({ ...done, title: "新しい名前" }, detailMark(), "keep"),
    );
    expect(result.current.state).toMatchObject({
      video: { title: "新しい名前", mediaInfo },
    });
    act(() =>
      result.current.replace({ ...done, title: "外した" }, detailMark(), "refetch"),
    );
    expect(result.current.state).toMatchObject({ video: { title: "外した", mediaInfo } });
  });

  it("replace の refetch は手元の作品情報を残さず、先に終わった取り直しの後でも取り直す", async () => {
    // 束から外す: 束の作品情報（BUNDLE）を出している動画が、外すと固有の作品情報（OWN）に戻る。
    const own: Video = { ...done, workInfo: workInfoCoded("OWN") };
    let answer: ((video: Video) => void) | undefined;
    getVideo
      .mockResolvedValueOnce({ ...done, workInfo: workInfoCoded("BUNDLE") })
      .mockResolvedValueOnce(own)
      .mockImplementationOnce(() => new Promise<Video>((resolve) => (answer = resolve)));
    const { result } = renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    // 外す要求を送り、その応答より先に `video` の知らせの取り直しが終わる。
    const mark = detailMark();
    await emitServerEvent("video", { id: 7 });
    await flush();
    expect(result.current.state).toMatchObject({ video: { workInfo: { code: "OWN" } } });

    // 外した応答は `workInfo` を載せない。手元の作品情報を残さず、取り直す。
    act(() => result.current.replace({ ...done, title: "外した" }, mark, "refetch"));
    const replaced = result.current.state;
    expect(
      replaced.kind === "ready" ? replaced.video.workInfo : "not ready",
    ).toBeUndefined();
    expect(getVideo).toHaveBeenCalledTimes(3);

    await act(async () => answer?.({ ...own, title: "外した" }));
    expect(result.current.state).toMatchObject({
      video: { title: "外した", workInfo: { code: "OWN" } },
    });
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
        "keep",
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

  it("最初の接続では、準備の済んだ動画を取り直さず、つなぎ直したら取り直す", async () => {
    getVideo.mockResolvedValue(done);
    renderHook(() => useVideoDetail(7), { wrapper: OwnerAudience });
    await flush();

    await emitServerEvent("open");
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(1);

    await emitServerEvent("open");
    await flush();
    expect(getVideo).toHaveBeenCalledTimes(2);
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
        new Response(JSON.stringify({ appliedVideos: 1, appliedGroups: 0 }), {
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
    expect(changes).toContainEqual({ videoIds: [11], groupIds: [], favorite: true });
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
          new Response(JSON.stringify({ appliedVideos: 1, appliedGroups: 0 }), {
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
    expect(changes).toEqual([{ videoIds: [12], groupIds: [], favorite: true }]);
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
        new Response(JSON.stringify({ appliedVideos: 1, appliedGroups: 0 }), {
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

    expect(changes).toEqual([{ videoIds: [11], groupIds: [], favorite: true }]);
    unsubscribe();
  });
});

describe("useRelatedVideos", () => {
  beforeEach(() => {
    installFakeEventSource();
    getRelatedVideos.mockReset();
  });

  it("並びのグループか開いたグループが変わった group の知らせで、並びを見せたまま最初のページから読み直す", async () => {
    const video = (id: number): Video => ({ ...done, id, title: `v${String(id)}` });
    const group = (items: Video[]) => ({
      id: 9,
      name: "picks",
      items,
      offset: 0,
      total: 2,
    });
    getRelatedVideos.mockResolvedValueOnce({
      items: [],
      group: group([video(7), video(8)]),
      nextId: 8,
    });
    const { result } = renderHook(() => useRelatedVideos(7, 4), {
      wrapper: OwnerAudience,
    });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(1);

    await emitServerEvent("group", { ids: [12] });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(1);

    let answer: (value: unknown) => void = () => undefined;
    getRelatedVideos.mockReset();
    getRelatedVideos.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          answer = resolve;
        }),
    );
    await emitServerEvent("group", { ids: [9] });
    expect(getRelatedVideos).toHaveBeenCalledWith(
      7,
      expect.anything(),
      undefined,
      4,
      undefined,
    );
    // 読み直しの間も今の並びを見せる。
    expect(result.current.state).toMatchObject({ kind: "ready", related: { nextId: 8 } });
    await act(async () => {
      answer({ items: [], group: group([video(8), video(7)]), prevId: 8 });
    });
    expect(result.current.state).toMatchObject({
      kind: "ready",
      related: { prevId: 8, group: { items: [{ id: 8 }, { id: 7 }] } },
      // 同じグループのまま読み直したので、グループの並びの画面を作り直す印が進む。
      generation: 2,
    });
  });

  it("smartFolder を渡し、開いたフォルダの知らせとどの group の知らせでも最初のページから読み直し、続きにも渡す", async () => {
    const video = (id: number): Video => ({ ...done, id, title: `v${String(id)}` });
    getRelatedVideos.mockResolvedValue({
      items: [video(1)],
      group: { id: 5, name: "タグ: 猫", items: [video(7)], offset: 0, total: 1 },
      nextCursor: "c1",
    });
    const { result } = renderHook(() => useRelatedVideos(7, undefined, 5), {
      wrapper: OwnerAudience,
    });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenLastCalledWith(
      7,
      expect.anything(),
      undefined,
      undefined,
      5,
    );
    await emitServerEvent("smartFolder", { ids: [6] });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(1);
    await emitServerEvent("smartFolder", { ids: [5] });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(2);
    await emitServerEvent("group", { ids: [99] });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(3);

    getRelatedVideos.mockResolvedValueOnce({ items: [video(2)] });
    act(() => result.current.loadMore());
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenLastCalledWith(
      7,
      expect.anything(),
      "c1",
      undefined,
      5,
    );
  });

  it("フォルダの文脈が無ければ smartFolder の知らせでは読み直さない", async () => {
    getRelatedVideos.mockResolvedValue({ items: [] });
    renderHook(() => useRelatedVideos(7), { wrapper: OwnerAudience });
    await act(async () => undefined);
    await emitServerEvent("smartFolder", { ids: [5] });
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenCalledTimes(1);
  });

  it("続きの取得中に group の知らせで読み直し、それが失敗しても、続きをもう一度読める", async () => {
    const video = (id: number): Video => ({ ...done, id, title: `v${String(id)}` });
    const picks = { id: 9, name: "picks", items: [video(7)], offset: 0, total: 1 };
    getRelatedVideos.mockResolvedValueOnce({
      items: [video(1)],
      group: picks,
      nextCursor: "c1",
    });
    const { result } = renderHook(() => useRelatedVideos(7), { wrapper: OwnerAudience });
    await act(async () => undefined);

    let signal: AbortSignal | undefined;
    getRelatedVideos.mockImplementationOnce(
      (_id: number, s: AbortSignal) =>
        new Promise((_resolve, reject) => {
          signal = s;
          s.addEventListener("abort", () => {
            reject(new DOMException("aborted", "AbortError"));
          });
        }),
    );
    act(() => result.current.loadMore());
    expect(result.current.state).toMatchObject({ more: "loading" });

    getRelatedVideos.mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"));
    await emitServerEvent("group", { ids: [9] });
    await act(async () => undefined);
    expect(signal?.aborted).toBe(true);
    // 読み直しは失敗したが、今の並びを残し、続きの読み込み中の印は外す。
    expect(result.current.state).toMatchObject({
      kind: "ready",
      related: { items: [{ id: 1 }], nextCursor: "c1" },
      more: "idle",
    });

    getRelatedVideos.mockResolvedValueOnce({ items: [video(2)] });
    act(() => result.current.loadMore());
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenLastCalledWith(
      7,
      expect.anything(),
      "c1",
      undefined,
      undefined,
    );
    expect(result.current.state).toMatchObject({
      related: { items: [{ id: 1 }, { id: 2 }] },
      more: "idle",
    });
  });

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

  it("loadMore は続きを nextCursor の後から読み、まだ無い動画だけを後ろに足す（issue 931）", async () => {
    const video = (id: number): Video => ({ ...done, id, title: `v${String(id)}` });
    getRelatedVideos
      .mockResolvedValueOnce({ items: [video(1), video(2)], nextId: 1, nextCursor: "c1" })
      .mockRejectedValueOnce(new RequestFailed(500, "internal", "失敗"))
      .mockResolvedValueOnce({ items: [video(2), video(3)], nextCursor: "c2" })
      .mockResolvedValueOnce({ items: [video(4)] });
    const { result } = renderHook(() => useRelatedVideos(7), { wrapper: OwnerAudience });
    await act(async () => undefined);
    const ids = () =>
      result.current.state.kind === "ready"
        ? result.current.state.related.items.map((item) => item.id)
        : [];

    // 失敗したら more が failed になり、もう一度 loadMore で読み直す。
    act(() => result.current.loadMore());
    expect(result.current.state).toMatchObject({ more: "loading" });
    await act(async () => undefined);
    expect(result.current.state).toMatchObject({ more: "failed" });
    act(() => result.current.loadMore());
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenNthCalledWith(
      3,
      7,
      expect.anything(),
      "c1",
      undefined,
      undefined,
    );
    expect(ids()).toEqual([1, 2, 3]);

    act(() => result.current.loadMore());
    await act(async () => undefined);
    expect(getRelatedVideos).toHaveBeenNthCalledWith(
      4,
      7,
      expect.anything(),
      "c2",
      undefined,
      undefined,
    );
    expect(ids()).toEqual([1, 2, 3, 4]);
    // 最初のページの次の動画は残し、続きが無くなったら読まない。
    expect(result.current.state).toMatchObject({ related: { nextId: 1 }, more: "idle" });
    expect(
      result.current.state.kind === "ready" && result.current.state.related.nextCursor,
    ).toBeUndefined();
    act(() => result.current.loadMore());
    expect(getRelatedVideos).toHaveBeenCalledTimes(4);
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
