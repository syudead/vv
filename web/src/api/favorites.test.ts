import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "./client";
import {
  favoriteMark,
  subscribeFavorites,
  subscribeFavoritesStale,
  updateFavorites,
  withFavoriteSince,
  withGroupFavoriteSince,
} from "./favorites";
import { videoItem } from "./libraryItems";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";

/**
 * お気に入りの付け外し（specs/035-favorites/contracts/screen-api.md §1、research.md R-6）。
 * 要求は 1 回だけ送り、受け付けた結果だけを画面と控えへ知らせる。
 */

function item(id: number, favorite = false): Video {
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
    favorite,
  };
}

function group(): LibraryGroup {
  return {
    folder: { rootId: 3, path: "series" },
    name: "series",
    videoCount: 2,
    durationMs: 0,
    sizeBytes: 0,
    addedAt: "2026-09-13T00:00:00Z",
    previews: [],
    openVideoId: 11,
    videoIds: [11, 12],
    tags: [],
    favorite: false,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const fetchMock = vi.fn<typeof fetch>();
const series = { rootId: 3, path: "series" };

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  clearListSnapshot();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("updateFavorites", () => {
  it("PUT /api/favorites を 1 回だけ送り、応答の後に知らせる", async () => {
    let answer: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (answer = resolve)),
    );
    const listener = vi.fn();
    const unsubscribe = subscribeFavorites(listener);

    const pending = updateFavorites([3], [series], true);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/favorites");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      videoIds: [3],
      folders: [series],
      favorite: true,
    });
    await Promise.resolve();
    expect(listener).not.toHaveBeenCalled();

    answer!(json({ appliedVideos: 1, appliedFolders: 1 }));
    await expect(pending).resolves.toEqual({ appliedVideos: 1, appliedFolders: 1 });
    expect(listener).toHaveBeenCalledWith({
      videoIds: [3],
      folders: [series],
      favorite: true,
    });
    unsubscribe();
  });

  it("失敗したら知らせず、控えも変えない", async () => {
    fetchMock.mockResolvedValue(json({ code: "internal", message: "x" }, 500));
    saveListSnapshot(
      { query: "" },
      { items: [videoItem(item(5))], total: 1, hasMore: false, scrollY: 0 },
    );
    const listener = vi.fn();
    const unsubscribe = subscribeFavorites(listener);
    await expect(updateFavorites([5], [], true)).rejects.toBeDefined();
    expect(listener).not.toHaveBeenCalled();
    const snapshot = takeListSnapshot({ query: "" });
    expect(snapshot?.items[0]).toEqual(videoItem(item(5)));
    unsubscribe();
  });

  it("控えの動画の favorite を差し替え、グループには取り直しの印を付ける", async () => {
    fetchMock.mockResolvedValue(json({ appliedVideos: 1, appliedFolders: 1 }));
    saveListSnapshot(
      { query: "" },
      {
        items: [videoItem(item(5)), { kind: "group", group: group() }],
        total: 2,
        hasMore: false,
        scrollY: 0,
      },
    );
    await updateFavorites([5], [series], true);
    const snapshot = takeListSnapshot({ query: "" });
    expect(snapshot?.items[0]).toEqual(videoItem(item(5, true)));
    // グループの値はそのままにし、戻ったときに取り直す。
    expect(snapshot?.items[1]).toEqual({ kind: "group", group: group() });
    expect(snapshot?.staleGroups).toEqual([series]);
  });

  it("一部の動画にしか反映されなければ、変えた扱いにせず取り直させる", async () => {
    fetchMock.mockResolvedValue(json({ appliedVideos: 1, appliedFolders: 0 }));
    const listener = vi.fn();
    const stale = vi.fn(() => undefined);
    const unsubscribe = subscribeFavorites(listener);
    const unsubscribeStale = subscribeFavoritesStale(stale);
    await updateFavorites([7, 8], [], true);
    expect(listener).not.toHaveBeenCalled();
    expect(stale).toHaveBeenCalledWith([7, 8]);
    unsubscribe();
    unsubscribeStale();
  });

  it("前の付け外しが決着してから次を送る", async () => {
    const answers: ((response: Response) => void)[] = [];
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => answers.push(resolve)),
    );
    const first = updateFavorites([9], [], true);
    const second = updateFavorites([9], [], false);
    await Promise.resolve();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    answers[0]!(json({ appliedVideos: 1, appliedFolders: 0 }));
    await first;
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    answers[1]!(json({ appliedVideos: 1, appliedFolders: 0 }));
    await second;
  });

  it("取得の前に印を取れば、その後の付け外しで取得した値を補正する", async () => {
    fetchMock.mockResolvedValue(json({ appliedVideos: 1, appliedFolders: 1 }));
    const mark = favoriteMark();
    await updateFavorites([21], [{ rootId: 4, path: "other" }], true);
    expect(withFavoriteSince(item(21), mark).favorite).toBe(true);
    const fetched = { ...group(), folder: { rootId: 4, path: "other" } };
    expect(withGroupFavoriteSince(fetched, mark).favorite).toBe(true);
    // 付け外しの後に取った値はそのまま。
    expect(withFavoriteSince(item(21), favoriteMark()).favorite).toBe(false);
  });
});
