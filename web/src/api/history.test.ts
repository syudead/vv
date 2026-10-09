import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { clearWatchHistory, deleteWatchHistoryEntry, listWatchHistory } from "./history";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("history API", () => {
  it("listWatchHistory は既定の 60 件で最初のページを取り、cursor を渡せば続きを取る", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json({ items: [], nextCursor: "c2" })),
    );
    vi.stubGlobal("fetch", fetchMock);

    expect(await listWatchHistory()).toEqual({ items: [], nextCursor: "c2" });
    await listWatchHistory({ cursor: "c 2", limit: 10 });

    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/watch-history?limit=60",
      "/api/watch-history?cursor=c+2&limit=10",
    ]);
  });

  it("deleteWatchHistoryEntry は 1 件を DELETE し、404 を RequestFailed で返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json({ code: "not_found", message: "x" }, 404));
    vi.stubGlobal("fetch", fetchMock);

    await deleteWatchHistoryEntry(7);
    const failure = await deleteWatchHistoryEntry(8).catch((error: unknown) => error);

    expect(
      fetchMock.mock.calls.map(([input, init]) => [String(input), init?.method]),
    ).toEqual([
      ["/api/watch-history/7", "DELETE"],
      ["/api/watch-history/8", "DELETE"],
    ]);
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).status).toBe(404);
  });

  it("clearWatchHistory は全件を DELETE し、失敗を RequestFailed で返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json({ code: "internal", message: "x" }, 500));
    vi.stubGlobal("fetch", fetchMock);

    await clearWatchHistory();
    await expect(clearWatchHistory()).rejects.toBeInstanceOf(RequestFailed);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/watch-history");
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("DELETE");
  });
});
