import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import {
  addToPlaylist,
  createPlaylist,
  deletePlaylist,
  getPlaylist,
  getPlaylistPlayback,
  listAllPlaylists,
  listPlaylists,
  MAX_LIST_RESTARTS,
  movePlaylistItem,
  removePlaylistItem,
  renamePlaylist,
} from "./playlists";

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

afterEach(() => {
  vi.unstubAllGlobals();
});

const summary = { id: 3, name: "朝", itemCount: 0, updatedAt: "2026-10-11T00:00:00Z" };

describe("playlists API", () => {
  it("listPlaylists・getPlaylist は 1 ページと 1 つを取り、一覧は limit と cursor を付ける", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ items: [summary], total: 2, version: "2.1", nextCursor: "c1" }),
      )
      .mockResolvedValueOnce(json({ items: [], total: 2, version: "2.1" }))
      .mockResolvedValueOnce(json({ ...summary, items: [] }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listPlaylists()).toEqual({
      items: [summary],
      total: 2,
      version: "2.1",
      nextCursor: "c1",
    });
    expect(await listPlaylists({ cursor: "c1", limit: 10 })).toEqual({
      items: [],
      total: 2,
      version: "2.1",
    });
    expect(await getPlaylist(3)).toEqual({ ...summary, items: [] });
    expect(sent(fetchMock).map(({ url, method }) => [url, method])).toEqual([
      ["/api/playlists?limit=60", "GET"],
      ["/api/playlists?cursor=c1&limit=10", "GET"],
      ["/api/playlists/3", "GET"],
    ]);
  });

  it("listAllPlaylists は nextCursor が尽きるまで最大の件数のページを辿り、全部を 1 つにする", async () => {
    const other = { ...summary, id: 4, name: "夜" };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ items: [summary], total: 2, version: "2.1", nextCursor: "c1" }),
      )
      .mockResolvedValueOnce(json({ items: [other], total: 2, version: "2.1" }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listAllPlaylists()).toEqual([summary, other]);
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/playlists?limit=200",
      "/api/playlists?cursor=c1&limit=200",
    ]);
  });

  it("listAllPlaylists は version が最初のページと違うページを捨て、最初のページから読み直す", async () => {
    const other = { ...summary, id: 4, name: "夜" };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ items: [summary], total: 2, version: "2.1", nextCursor: "c1" }),
      )
      .mockResolvedValueOnce(json({ items: [], total: 2, version: "2.2" }))
      .mockResolvedValueOnce(
        json({ items: [other], total: 2, version: "2.2", nextCursor: "c2" }),
      )
      .mockResolvedValueOnce(json({ items: [summary], total: 2, version: "2.2" }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listAllPlaylists()).toEqual([other, summary]);
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/playlists?limit=200",
      "/api/playlists?cursor=c1&limit=200",
      "/api/playlists?limit=200",
      "/api/playlists?cursor=c2&limit=200",
    ]);
  });

  it("listAllPlaylists は一覧が変わり続けたら、欠けた一覧を返さずに失敗する", async () => {
    let calls = 0;
    const fetchMock = vi.fn<typeof fetch>(() => {
      calls += 1;
      return Promise.resolve(
        json({ items: [summary], total: 2, version: String(calls), nextCursor: "c" }),
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(listAllPlaylists()).rejects.toThrow(/kept changing/);
    expect(calls).toBe(2 * (MAX_LIST_RESTARTS + 1));
  });

  it("createPlaylist・renamePlaylist は名前を JSON で送り、400 を RequestFailed で返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(summary, 201))
      .mockResolvedValueOnce(json({ ...summary, name: "夜" }))
      .mockResolvedValueOnce(json({ code: "invalid_request", message: "x" }, 400));
    vi.stubGlobal("fetch", fetchMock);

    expect(await createPlaylist(" 朝 ")).toEqual(summary);
    expect((await renamePlaylist(3, "夜")).name).toBe("夜");
    const failure = await createPlaylist("").catch((error: unknown) => error);

    expect(sent(fetchMock)).toEqual([
      { url: "/api/playlists", method: "POST", body: { name: " 朝 " } },
      { url: "/api/playlists/3", method: "PATCH", body: { name: "夜" } },
      { url: "/api/playlists", method: "POST", body: { name: "" } },
    ]);
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toEqual({
      "Content-Type": "application/json",
    });
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).status).toBe(400);
  });

  it("addToPlaylist は videoIds と groupIds を送り、added と existing を返す", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json({ added: 2, existing: 1 })),
    );
    vi.stubGlobal("fetch", fetchMock);

    expect(await addToPlaylist(3, [1, 2], [4])).toEqual({
      added: 2,
      existing: 1,
    });
    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/playlists/3/items",
        method: "POST",
        body: { videoIds: [1, 2], groupIds: [4] },
      },
    ]);
  });

  it("movePlaylistItem は afterItemId（null は先頭）を送り、項目と削除を DELETE する", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json({ code: "not_found", message: "x" }, 404))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await movePlaylistItem(3, 7, null);
    await movePlaylistItem(3, 7, 8);
    await removePlaylistItem(3, 7);
    const failure = await removePlaylistItem(3, 7).catch((error: unknown) => error);
    await deletePlaylist(3);

    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/playlists/3/items/7/move",
        method: "POST",
        body: { afterItemId: null },
      },
      { url: "/api/playlists/3/items/7/move", method: "POST", body: { afterItemId: 8 } },
      { url: "/api/playlists/3/items/7", method: "DELETE", body: undefined },
      { url: "/api/playlists/3/items/7", method: "DELETE", body: undefined },
      { url: "/api/playlists/3", method: "DELETE", body: undefined },
    ]);
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).status).toBe(404);
  });

  it("getPlaylistPlayback は項目と動画を送り、位置と次の動画を返す", async () => {
    const body = {
      playlist: { id: 4, name: "朝", itemCount: 3, updatedAt: "2026-10-11T00:00:00Z" },
      itemId: 7,
      position: 1,
      count: 3,
    };
    const fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(json(body)));
    vi.stubGlobal("fetch", fetchMock);

    expect(await getPlaylistPlayback(4, 7, 12)).toEqual(body);
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/playlists/4/playback?item=7&video=12",
    ]);
  });

  it("getPlaylistPlayback はプレイリストか項目が無ければ 404 の RequestFailed になる", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(() =>
        Promise.resolve(
          json({ code: "not_found", message: "Playlist item not found." }, 404),
        ),
      ),
    );

    const error: unknown = await getPlaylistPlayback(4, 7, 12).catch(
      (caught: unknown) => caught,
    );
    expect(error).toBeInstanceOf(RequestFailed);
    expect(error).toMatchObject({ status: 404, code: "not_found" });
  });
});
