import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createMediaFolder,
  deleteMediaFolder,
  fetchSeekThumbnail,
  listDirectories,
  listFolderVideos,
  listVideos,
  beaconProgress,
  listMediaFolders,
  startScan,
  transcodeUrl,
  saveProgress,
  updateMediaFolder,
} from "./client";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";

describe("playback URLs", () => {
  it("transcode startMsを省略または整数化する", () => {
    expect(transcodeUrl(7)).toBe("/api/videos/7/transcode.mp4");
    expect(transcodeUrl(7, 12_345.4)).toBe("/api/videos/7/transcode.mp4?startMs=12345");
  });

  it("fetches seek thumbnails through the API boundary", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(
      new Response(new Uint8Array([1, 2, 3]), {
        headers: { "Content-Type": "image/jpeg" },
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;

    const image = await fetchSeekThumbnail(
      "/api/videos/7/seek-thumbnail?positionMs=5000",
      signal,
    );
    expect(image).toMatchObject({ size: 3, type: "image/jpeg" });
    expect(fetch).toHaveBeenCalledWith("/api/videos/7/seek-thumbnail?positionMs=5000", {
      signal,
    });
  });

  it("rejects unavailable seek thumbnails", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof globalThis.fetch>()
        .mockResolvedValue(new Response(null, { status: 409 })),
    );

    await expect(
      fetchSeekThumbnail(
        "/api/videos/7/seek-thumbnail?positionMs=5000",
        new AbortController().signal,
      ),
    ).rejects.toMatchObject({ status: 409, code: "seek_thumbnail_failed" });
  });
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("settings API client", () => {
  it("sends individual folder mutations with generated contract shapes", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(jsonResponse({ id: 1, path: "/a", version: 1 }))
      .mockResolvedValueOnce(jsonResponse({ id: 1, path: "/b", version: 2 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(
        jsonResponse({ id: 1, state: "running", total: 0, completed: 0, failed: 0 }),
      );
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;

    await listMediaFolders(signal);
    await createMediaFolder("/a", signal);
    await updateMediaFolder(1, "/b", 1, signal);
    await deleteMediaFolder(1, 2, signal);
    await startScan(signal);

    expect(fetch).toHaveBeenNthCalledWith(1, "/api/media-folders", { signal });
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/media-folders",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ path: "/a" }),
        signal,
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/media-folders/1",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ path: "/b", version: 1 }),
        signal,
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/media-folders/1?version=2", {
      method: "DELETE",
      signal,
    });
    expect(fetch).toHaveBeenNthCalledWith(
      5,
      "/api/scans",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({}),
        signal,
      }),
    );
  });

  it("登録フォルダの追加・変更・削除が成功したら一覧の控えを捨て、失敗したら残す", async () => {
    const key = { query: "", sort: "addedDesc" as const, folder: "3\0A" };
    const hold = () =>
      saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const fetch = vi.fn<typeof globalThis.fetch>();
    vi.stubGlobal("fetch", fetch);

    fetch.mockResolvedValueOnce(jsonResponse({ id: 1, path: "/a", version: 1 }));
    hold();
    await createMediaFolder("/a");
    expect(takeListSnapshot(key)).toBeUndefined();

    fetch.mockResolvedValueOnce(jsonResponse({ id: 1, path: "/b", version: 2 }));
    hold();
    await updateMediaFolder(1, "/b", 1);
    expect(takeListSnapshot(key)).toBeUndefined();

    fetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    hold();
    await deleteMediaFolder(1, 2);
    expect(takeListSnapshot(key)).toBeUndefined();

    fetch.mockResolvedValueOnce(
      jsonResponse({ code: "version_conflict", message: "conflict" }, 409),
    );
    hold();
    await expect(deleteMediaFolder(1, 1)).rejects.toThrow();
    expect(takeListSnapshot(key)).toBeDefined();
  });

  it("encodes directory paths and forwards cancellation", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        jsonResponse({ currentPath: "C:\\Media Files", directories: [] }),
      );
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;

    await listDirectories("C:\\Media Files", signal);

    expect(fetch).toHaveBeenCalledWith("/api/directories?path=C%3A%5CMedia+Files", {
      signal,
    });
  });
});

describe("progress API client", () => {
  const key = { query: "", sort: "addedDesc" as const };
  const progress = {
    positionMs: 60_000,
    completed: true,
    updatedAt: "2026-09-23T00:00:00Z",
  };
  const unwatched = {
    id: 7,
    title: "x",
    sizeBytes: 1,
    addedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done" as const,
    thumbnailState: "done" as const,
    previewState: "pending" as const,
  };

  it("保存と離脱時の送信が受け付けられたら、一覧の控えの再生位置を差し替える", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>();
    vi.stubGlobal("fetch", fetch);

    saveListSnapshot(key, { items: [unwatched], total: 1, hasMore: false, scrollY: 0 });
    fetch.mockResolvedValueOnce(jsonResponse(progress));
    await saveProgress(7, 60_000);
    expect(takeListSnapshot(key)?.items[0]?.progress).toEqual(progress);

    saveListSnapshot(key, { items: [unwatched], total: 1, hasMore: false, scrollY: 0 });
    fetch.mockResolvedValueOnce(jsonResponse(progress));
    beaconProgress(7, 60_000);
    await vi.waitFor(() => {
      expect(takeListSnapshot(key)?.items[0]?.progress).toEqual(progress);
    });

    // ページが隠れるときの送信は待たずに出るので、応答の順が入れ替わりうる。
    // その場合も、後から送った保存の位置を残す。
    saveListSnapshot(key, { items: [unwatched], total: 1, hasMore: false, scrollY: 0 });
    const older = {
      positionMs: 10_000,
      completed: false,
      updatedAt: "2026-09-23T00:00:00Z",
    };
    const newer = {
      positionMs: 20_000,
      completed: false,
      updatedAt: "2026-09-23T00:00:00Z",
    };
    let answerOlder: (response: Response) => void = () => undefined;
    const callsBefore = fetch.mock.calls.length;
    fetch.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          answerOlder = resolve;
        }),
    );
    fetch.mockResolvedValueOnce(jsonResponse(newer));
    const first = saveProgress(7, 10_000);
    // 保存は順番待ちを経て送られるので、送り始めてから次へ進む。
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(callsBefore + 1));
    const hidden = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    beaconProgress(7, 20_000);
    hidden.mockRestore();
    await vi.waitFor(() => {
      expect(takeListSnapshot(key)?.items[0]?.progress).toEqual(newer);
    });
    answerOlder(jsonResponse(older));
    await first;
    expect(takeListSnapshot(key)?.items[0]?.progress).toEqual(newer);

    // 失敗した送信では控えを書き換えない。
    saveListSnapshot(key, { items: [unwatched], total: 1, hasMore: false, scrollY: 0 });
    fetch.mockResolvedValueOnce(jsonResponse({ code: "not_found", message: "x" }, 404));
    beaconProgress(7, 60_000);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(takeListSnapshot(key)?.items[0]?.progress).toBeUndefined();
  });

  it("同じ動画の保存は、前の保存が終わってから送る", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>();
    vi.stubGlobal("fetch", fetch);
    let answerFirst: (response: Response) => void = () => undefined;
    fetch.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          answerFirst = resolve;
        }),
    );
    fetch.mockImplementation(() => Promise.resolve(jsonResponse(progress)));

    const first = saveProgress(8, 10_000);
    const second = saveProgress(8, 20_000);
    beaconProgress(8, 30_000);
    // 別の動画は待たない。
    await saveProgress(9, 5_000);
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(fetch.mock.calls.map(([url]) => String(url))).toEqual([
      "/api/videos/8/progress",
      "/api/videos/9/progress",
    ]);

    answerFirst(jsonResponse(progress));
    await first;
    await second;
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(4));
    const bodies = fetch.mock.calls
      .filter(([url]) => String(url) === "/api/videos/8/progress")
      .map(([, init]) => JSON.parse(String(init?.body)) as { positionMs: number });
    expect(bodies.map((body) => body.positionMs)).toEqual([10_000, 20_000, 30_000]);
  });

  it("ページが隠れるときの送信は待たずに出し、以後の保存はその完了を待つ", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>();
    vi.stubGlobal("fetch", fetch);
    let answerBeacon: (response: Response) => void = () => undefined;
    fetch.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          answerBeacon = resolve;
        }),
    );
    fetch.mockImplementation(() => Promise.resolve(jsonResponse(progress)));

    const hidden = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    beaconProgress(10, 20_000);
    hidden.mockRestore();
    expect(fetch).toHaveBeenCalledTimes(1);

    // タブが再び表示されて始まった保存は、隠れたときの送信の完了を待つ。
    const next = saveProgress(10, 25_000);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetch).toHaveBeenCalledTimes(1);

    answerBeacon(jsonResponse(progress));
    await next;
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});

describe("list API client", () => {
  const emptyPage = { items: [], total: 0 };

  function stubFetch() {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockImplementation(() => Promise.resolve(jsonResponse(emptyPage)));
    vi.stubGlobal("fetch", fetch);
    return fetch;
  }

  function requestedURL(fetch: ReturnType<typeof stubFetch>, call = 0): URL {
    return new URL(String(fetch.mock.calls[call]?.[0]), "http://localhost");
  }

  it("keeps the existing library query when no new conditions are given", async () => {
    const fetch = stubFetch();
    await listVideos({ query: "京都", sort: "titleAsc", cursor: "c" });
    expect(fetch.mock.calls[0]?.[0]).toBe(
      "/api/videos?query=%E4%BA%AC%E9%83%BD&sort=titleAsc&cursor=c&limit=60",
    );
  });

  it("sends watch, playable and seed to the library list", async () => {
    const fetch = stubFetch();
    await listVideos({
      query: "京都 -2023",
      watch: "unwatched",
      playable: true,
      sort: "random",
      seed: 42,
      limit: 10,
    });
    const url = requestedURL(fetch);
    expect(url.pathname).toBe("/api/videos");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      query: "京都 -2023",
      watch: "unwatched",
      playable: "true",
      sort: "random",
      seed: "42",
      limit: "10",
    });
  });

  it("omits playable when it is false", async () => {
    const fetch = stubFetch();
    await listVideos({ playable: false });
    expect(requestedURL(fetch).searchParams.has("playable")).toBe(false);
  });

  it("sends scope, query and filters to the folder list", async () => {
    const fetch = stubFetch();
    await listFolderVideos({
      folder: { rootId: 3, path: "A/B" },
      scope: "subtree",
      query: "京都",
      watch: "inProgress",
      playable: true,
      sort: "durationDesc",
      seed: 0,
    });
    const url = requestedURL(fetch);
    expect(url.pathname).toBe("/api/folders/3/videos");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      path: "A/B",
      scope: "subtree",
      query: "京都",
      watch: "inProgress",
      playable: "true",
      sort: "durationDesc",
      seed: "0",
      limit: "60",
    });
  });

  it("keeps the existing folder query when no new conditions are given", async () => {
    const fetch = stubFetch();
    await listFolderVideos({ folder: { rootId: 3, path: "" }, sort: "addedDesc" });
    expect(fetch.mock.calls[0]?.[0]).toBe(
      "/api/folders/3/videos?sort=addedDesc&limit=60",
    );
  });

  it("truncates an overlong folder query to the contract limit", async () => {
    const fetch = stubFetch();
    await listFolderVideos({ folder: { rootId: 3, path: "" }, query: "あ".repeat(150) });
    expect(requestedURL(fetch).searchParams.get("query")).toHaveLength(100);
  });
});
