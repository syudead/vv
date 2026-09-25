import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "./client";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import { subscribeVideoVisibility, updateVideoVisibility } from "./visibility";

/**
 * 公開・非公開の切り替え（specs/016-single-account-auth/contracts/guest-api.md §4、
 * issue 305）。要求は1回だけ送り、受け付けた結果だけを画面と控えへ知らせる。
 */

function item(id: number, isPublic = false): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: isPublic,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
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
    // 応答の前は知らせない。
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
      { items: [item(1)], total: 1, hasMore: false, scrollY: 0 },
    );
    const listener = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);

    await expect(updateVideoVisibility([1], true)).rejects.toBeDefined();
    expect(listener).not.toHaveBeenCalled();
    expect(takeListSnapshot({ query: "" })?.items[0]?.public).toBe(false);
    unsubscribe();
  });

  it("受け付けた結果を一覧の控えの public へ重ねる", async () => {
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    saveListSnapshot(
      { query: "" },
      { items: [item(1), item(2)], total: 2, hasMore: false, scrollY: 0 },
    );
    await updateVideoVisibility([2], true);
    expect(takeListSnapshot({ query: "" })?.items.map((video) => video.public)).toEqual([
      false,
      true,
    ]);
  });

  it("古い応答が後から届いても、同じ動画の新しい結果を巻き戻さない", async () => {
    const answers: ((response: Response) => void)[] = [];
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => answers.push(resolve)),
    );
    const seen: boolean[] = [];
    const unsubscribe = subscribeVideoVisibility((_, isPublic) => seen.push(isPublic));

    const first = updateVideoVisibility([9], true);
    const second = updateVideoVisibility([9], false);
    answers[1]!(json({ applied: 1 }));
    await second;
    answers[0]!(json({ applied: 1 }));
    await first;

    expect(seen).toEqual([false]);
    unsubscribe();
  });
});
