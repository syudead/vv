import { afterEach, describe, expect, it, vi } from "vitest";

import {
  getRelatedVideos,
  getVideo,
  listVideoGroupMembers,
  RequestFailed,
} from "./client";
import { updateFavorites } from "./favorites";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import { addToPlaylist, getPlaylist } from "./playlists";
import {
  changeSmartFolder,
  createSmartFolder,
  deleteSmartFolder,
  listSmartFolderItems,
  listSmartFolders,
  readSmartFolderCards,
  readSmartFolderPage,
} from "./smartFolders";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function sent(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>) {
  return fetchMock.mock.calls.map(([input, init]) => ({
    url: String(input),
    method: init?.method ?? "GET",
    body: typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined,
  }));
}

function holdSnapshot() {
  saveListSnapshot({ query: "" }, { items: [], total: 0, hasMore: false, scrollY: 0 });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const summary = { id: 3, name: "猫" };
const card = { id: 3, name: "猫", videoCount: 2, sizeBytes: 10, previews: [], tags: [] };

describe("smart folders API", () => {
  it("reads the list, a page, the next page and the cards", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ smartFolders: [summary] }))
      .mockResolvedValueOnce(
        json({ smartFolder: card, trail: [], items: [], nextCursor: "c1" }),
      )
      .mockResolvedValueOnce(json({ items: [] }))
      .mockResolvedValueOnce(json({ cards: [card] }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listSmartFolders()).toEqual([summary]);
    expect((await readSmartFolderPage(3, undefined, 50)).nextCursor).toBe("c1");
    expect(await listSmartFolderItems(3, "c1")).toEqual({ items: [] });
    expect(await readSmartFolderCards([3, 9])).toEqual([card]);
    expect(await readSmartFolderCards([])).toEqual([]);
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/smart-folders",
      "/api/smart-folders/3?limit=50",
      "/api/smart-folders/3/items?cursor=c1",
      "/api/smart-folders/cards?ids=3,9",
    ]);
  });

  it("sends writes as JSON, drops the list snapshot and returns failures as RequestFailed", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(summary, 201))
      .mockResolvedValueOnce(json({ code: "smart_folder_cycle", message: "x" }, 409))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    expect(
      await createSmartFolder({
        name: "猫",
        rule: { tags: [1], favorite: true },
        parentId: 2,
      }),
    ).toEqual(summary);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    holdSnapshot();
    const failure = await changeSmartFolder(3, { parentId: 4 }).catch(
      (error: unknown) => error,
    );
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).code).toBe("smart_folder_cycle");
    expect(takeListSnapshot({ query: "" })).toBeDefined();

    holdSnapshot();
    await deleteSmartFolder(3);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/smart-folders",
        method: "POST",
        body: { name: "猫", rule: { tags: [1], favorite: true }, parentId: 2 },
      },
      { url: "/api/smart-folders/3", method: "PATCH", body: { parentId: 4 } },
      { url: "/api/smart-folders/3", method: "DELETE", body: undefined },
    ]);
  });

  it("sends limit only when given and drops the snapshot when the folder to delete is gone", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ smartFolder: card, trail: [], items: [] }))
      .mockResolvedValueOnce(json({ items: [] }))
      .mockResolvedValueOnce(json({ code: "not_found", message: "x" }, 404));
    vi.stubGlobal("fetch", fetchMock);

    await readSmartFolderPage(3);
    await listSmartFolderItems(3, "c1", undefined, 20);
    holdSnapshot();
    const failure = await deleteSmartFolder(3).catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).code).toBe("not_found");
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/smart-folders/3",
      "/api/smart-folders/3/items?cursor=c1&limit=20",
      "/api/smart-folders/3",
    ]);
  });

  it("sends smartFolderIds to favorites and playlists only when there are some", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 1 }),
      )
      .mockResolvedValueOnce(
        json({ appliedVideos: 1, appliedGroups: 0, appliedSmartFolders: 0 }),
      )
      .mockResolvedValueOnce(json({ added: 1, existing: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await updateFavorites([], [], true, { smartFolderIds: [3] });
    await updateFavorites([1], [], false);
    await addToPlaylist(5, [], [], undefined, [3]);
    expect(sent(fetchMock).map(({ body }) => body)).toEqual([
      { videoIds: [], groupIds: [], smartFolderIds: [3], favorite: true },
      { videoIds: [1], groupIds: [], favorite: false },
      { videoIds: [], groupIds: [], smartFolderIds: [3] },
    ]);
  });

  it("keeps smart folder items of a playlist in the server's order", async () => {
    const video = { id: 1, kind: "video", video: { id: 1, title: "a" } };
    const folder = { id: 2, kind: "smartFolder", smartFolder: card };
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValueOnce(
        json({
          id: 5,
          name: "P",
          itemCount: 2,
          updatedAt: "2026-10-16T00:00:00Z",
          items: [video, folder],
        }),
      ),
    );
    expect((await getPlaylist(5)).items).toEqual([video, folder]);
  });

  it("passes the smart folder a video was opened from to the video reads", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation(() =>
        Promise.resolve(json({ id: 1, items: [], offset: 0, total: 1 })),
      );
    vi.stubGlobal("fetch", fetchMock);

    await getVideo(1, undefined, undefined, 3);
    await getRelatedVideos(1, undefined, undefined, undefined, 3);
    await listVideoGroupMembers(1, 0, 10, undefined, 2, 3);
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/videos/1?smartFolder=3",
      "/api/videos/1/related?smartFolder=3",
      "/api/videos/1/group-members?offset=0&limit=10&group=2&smartFolder=3",
    ]);
  });
});
