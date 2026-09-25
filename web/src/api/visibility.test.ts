import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "./client";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  subscribeVideoVisibility,
  subscribeVideoVisibilityStale,
  updateVideoVisibility,
  visibilityMark,
  withVisibilitySince,
} from "./visibility";

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
    // 2つを同時に送ると、サーバーに届く順が押した順と入れ替わりうる。
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

    // 最後に押した「非公開」が最後に反映される。
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
    // 空の content_key の動画やライブラリから消えた id は数えない（guest-api.md §4）。
    fetchMock.mockResolvedValue(json({ applied: 1 }));
    saveListSnapshot(
      { query: "" },
      { items: [item(21), item(22)], total: 2, hasMore: false, scrollY: 0 },
    );
    const listener = vi.fn();
    const stale = vi.fn();
    const unsubscribe = subscribeVideoVisibility(listener);
    const unsubscribeStale = subscribeVideoVisibilityStale(stale);
    const before = visibilityMark();

    await expect(updateVideoVisibility([21, 22], true)).resolves.toEqual({ applied: 1 });

    expect(listener).not.toHaveBeenCalled();
    expect(stale).toHaveBeenCalledWith([21, 22]);
    // どれが切り替わったか分からない控えは、再生画面から戻ったときに復元させない。
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    // 取り直した内容をそのまま使う。
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

    // 切り替えの前に始めた取得は、切り替える前の値を読んでいることがある。
    expect(withVisibilitySince(item(11), before).public).toBe(true);
    // 切り替えの後に始めた取得は、サーバーの値をそのまま使う。
    expect(withVisibilitySince(item(11), after).public).toBe(false);
    // 切り替えていない動画は変えない。
    const other = item(12);
    expect(withVisibilitySince(other, before)).toBe(other);
  });
});
