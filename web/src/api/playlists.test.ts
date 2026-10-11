import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { getPlaylistPlayback } from "./playlists";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("playlists API", () => {
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
