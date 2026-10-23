import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "./client";
import { itemVideos, videoItem } from "./libraryItems";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  subscribeVideoVisibility,
  subscribeVideoVisibilityStale,
  updateVideoVisibility,
  visibilityMark,
  withVisibilitySince,
} from "./visibility";

function item(id: number, isPublic = false): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: isPublic,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    updatedAt: "2026-09-13T00:00:00Z",
    fileCreatedAt: "2026-09-13T00:00:00Z",
    fileModifiedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  clearListSnapshot();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("updateVideoVisibility", () => {
  it("PUT /api/video-visibility を1回だけ送り、応答の後に知らせる", async () => {
    let answer: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (answer = resolve)),
    );
    const listener = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);

    const pending = updateVideoVisibility([3, 4], true);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/video-visibility");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ videoIds: [3, 4], public: true });
    await Promise.resolve();
    expect(listener).not.toHaveBeenCalled();

    answer!(json({ applied: 2 }));
    await expect(pending).resolves.toEqual({ applied: 2 });
    expect(listener).toHaveBeenCalledWith([3, 4], true);
    unsubscribe();
  });

  it("失敗したら知らせず、控えも変えない", async () => {
    fetchMock.mockResolvedValue(json({ code: "internal", message: "失敗" }, 500));
    saveListSnapshot(
      { query: "" },
      { items: [videoItem(item(1))], total: 1, hasMore: false, scrollY: 0 },
    );
    const listener = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);

    await expect(updateVideoVisibility([1], true)).rejects.toBeDefined();
    expect(listener).not.toHaveBeenCalled();
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.public).toBe(
      false,
    );
    unsubscribe();
  });

  it("受け付けた結果を一覧の控えの public へ重ねる", async () => {
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    saveListSnapshot(
      { query: "" },
      {
        items: [videoItem(item(1)), videoItem(item(2))],
        total: 2,
        hasMore: false,
        scrollY: 0,
      },
    );
    await updateVideoVisibility([2], true);
    expect(
      itemVideos(takeListSnapshot({ query: "" })?.items ?? []).map(
        (video) => video.public,
      ),
    ).toEqual([false, true]);
  });

  it("前の切り替えが決着するまで次を送らず、押した順にサーバーへ届ける", async () => {
    const answers: ((response: Response) => void)[] = [];
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => answers.push(resolve)),
    );
    const seen: boolean[] = [];
    const unsubscribe = subscribeVideoVisibility((_, isPublic) => seen.push(isPublic));

    const first = updateVideoVisibility([9], true);
    const second = updateVideoVisibility([9], false);
    await Promise.resolve();
    // Concurrent requests can reach the server in a different order from user actions.
    expect(fetchMock).toHaveBeenCalledOnce();

    answers[0]!(json({ applied: 1 }));
    await first;
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(JSON.parse(String(fetchMock.mock.calls[1]![1]?.body))).toEqual({
      videoIds: [9],
      public: false,
    });
    answers[1]!(json({ applied: 1 }));
    await second;

    expect(seen).toEqual([true, false]);
    unsubscribe();
  });

  it("前の切り替えが失敗しても、次の切り替えは送る", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "internal", message: "失敗" }, 500))
      .mockResolvedValueOnce(json({ applied: 1 }));
    const first = updateVideoVisibility([5], true);
    const second = updateVideoVisibility([5], true);
    await expect(first).rejects.toBeDefined();
    await expect(second).resolves.toEqual({ applied: 1 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe("一部にしか反映されなかった切り替え", () => {
  it("applied が異なる id の数より少なければ切り替え済みにせず、控えを捨て、取り直しを求める", async () => {
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    saveListSnapshot(
      { query: "" },
      {
        items: [videoItem(item(21)), videoItem(item(22))],
        total: 2,
        hasMore: false,
        scrollY: 0,
      },
    );
    const listener = vi.fn();
    const stale = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);
    const unsubscribeStale = subscribeVideoVisibilityStale(stale);
    const before = visibilityMark();

    await expect(updateVideoVisibility([21, 22], true)).resolves.toEqual({ applied: 1 });

    expect(listener).not.toHaveBeenCalled();
    expect(stale).toHaveBeenCalledWith([21, 22]);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    expect(withVisibilitySince(item(21), before).public).toBe(false);
    unsubscribe();
    unsubscribeStale();
  });

  it("applied が 0 の1本は切り替え済みにしない", async () => {
    fetchMock.mockResolvedValue(json({ applied: 0 }));
    const listener = vi.fn();
    const stale = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);
    const unsubscribeStale = subscribeVideoVisibilityStale(stale);

    await updateVideoVisibility([23], true);

    expect(listener).not.toHaveBeenCalled();
    expect(stale).toHaveBeenCalledWith([23]);
    unsubscribe();
    unsubscribeStale();
  });

  it("重複した id は1つとして数える", async () => {
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    const listener = vi.fn();
    const stale = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);
    const unsubscribeStale = subscribeVideoVisibilityStale(stale);

    await updateVideoVisibility([24, 24], true);

    expect(listener).toHaveBeenCalledWith([24], true);
    expect(stale).not.toHaveBeenCalled();
    unsubscribe();
    unsubscribeStale();
  });
});

describe("withVisibilitySince", () => {
  it("印を取った後に反映した切り替えだけを、取得した動画に重ねる", async () => {
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    const before = visibilityMark();
    await updateVideoVisibility([11], true);
    const after = visibilityMark();

    expect(withVisibilitySince(item(11), before).public).toBe(true);
    expect(withVisibilitySince(item(11), after).public).toBe(false);
    const other = item(12);
    expect(withVisibilitySince(other, before)).toBe(other);
  });
});
