import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OwnerAudience } from "../testing/audience";
import { RequestFailed, type Video } from "./client";
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

const { isProcessing, useRelatedVideos, useVideoDetail } =
  await import("./useVideoDetail");

const done: Video = {
  id: 7,
  title: "動画",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
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
      reason: "壊れています",
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
});

describe("useRelatedVideos", () => {
  beforeEach(() => getRelatedVideos.mockReset());

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
