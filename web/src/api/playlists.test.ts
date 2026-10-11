import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import {
  addToPlaylist,
  createPlaylist,
  deletePlaylist,
  getPlaylist,
  listPlaylists,
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
  it("listPlaylists・getPlaylist は一覧と 1 つを取り、一覧は items を返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ items: [summary] }))
      .mockResolvedValueOnce(json({ ...summary, items: [] }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listPlaylists()).toEqual([summary]);
    expect(await getPlaylist(3)).toEqual({ ...summary, items: [] });
    expect(sent(fetchMock).map(({ url, method }) => [url, method])).toEqual([
      ["/api/playlists", "GET"],
      ["/api/playlists/3", "GET"],
    ]);
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

  it("addToPlaylist は videoIds と folders を送り、added と existing を返す", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json({ added: 2, existing: 1 })),
    );
    vi.stubGlobal("fetch", fetchMock);

    expect(await addToPlaylist(3, [1, 2], [{ rootId: 4, path: "show" }])).toEqual({
      added: 2,
      existing: 1,
    });
    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/playlists/3/items",
        method: "POST",
        body: { videoIds: [1, 2], folders: [{ rootId: 4, path: "show" }] },
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
});
