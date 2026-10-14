import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import { moveCards, reorderGroup, undoOrganize } from "./organize";

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

/** holdSnapshot はライブラリの一覧の控えを 1 つ置く（動画ページへ離れたときと同じ）。 */
function holdSnapshot() {
  saveListSnapshot({ query: "" }, { items: [], total: 0, hasMore: false, scrollY: 0 });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("organize API", () => {
  it("reorderGroup は項目を画面の順に送り、after が null なら送らない。トークンを返し、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t1" }))
      .mockResolvedValueOnce(json({ undoToken: "t2" }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await expect(reorderGroup(7, [13, 11], 12)).resolves.toEqual({ undoToken: "t1" });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    await expect(reorderGroup(7, [13], null)).resolves.toEqual({ undoToken: "t2" });

    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/organize/reorder",
        method: "POST",
        body: { groupId: 7, itemIds: [13, 11], after: 12 },
      },
      {
        url: "/api/organize/reorder",
        method: "POST",
        body: { groupId: 7, itemIds: [13] },
      },
    ]);
  });

  it("moveCards はカードを画面の順に送り、null の from と to は送らない。トークンを返し、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t1" }))
      .mockResolvedValueOnce(json({ undoToken: "t2" }))
      .mockResolvedValueOnce(json({ undoToken: "t3" }));
    vi.stubGlobal("fetch", fetchMock);

    const items = [
      { kind: "group" as const, id: 4 },
      { kind: "video" as const, id: 9 },
    ];
    holdSnapshot();
    await expect(moveCards(items, { from: 2, to: 3 })).resolves.toEqual({
      undoToken: "t1",
    });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    await expect(moveCards(items, { from: 2, to: null })).resolves.toEqual({
      undoToken: "t2",
    });
    await expect(moveCards(items, { from: null, to: 3 })).resolves.toEqual({
      undoToken: "t3",
    });

    expect(sent(fetchMock)).toEqual([
      { url: "/api/organize/move", method: "POST", body: { items, from: 2, to: 3 } },
      { url: "/api/organize/move", method: "POST", body: { items, from: 2 } },
      { url: "/api/organize/move", method: "POST", body: { items, to: 3 } },
    ]);
  });

  it("moveCards の 409 group_cycle は RequestFailed になり、一覧の控えを残す", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(
          json({ code: "conflict", reason: "group_cycle", message: "cycle" }, 409),
        ),
    );

    holdSnapshot();
    await expect(
      moveCards([{ kind: "group", id: 1 }], { from: null, to: 2 }),
    ).rejects.toMatchObject({
      status: 409,
      reason: "group_cycle",
    });
    expect(takeListSnapshot({ query: "" })).toBeDefined();
  });

  it("undoOrganize はトークンを送って 204 で終わり、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await undoOrganize("t1");
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    expect(sent(fetchMock)).toEqual([
      { url: "/api/organize/undo", method: "POST", body: { token: "t1" } },
    ]);
  });

  it("失敗は reason 付きの RequestFailed で返し、一覧の控えを残す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ code: "not_found", reason: "group_target_not_found", message: "x" }, 404),
      )
      .mockResolvedValueOnce(
        json({ code: "conflict", reason: "undo_conflict", message: "x" }, 409),
      )
      .mockResolvedValueOnce(
        json({ code: "not_found", reason: "undo_expired", message: "x" }, 410),
      );
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await expect(reorderGroup(7, [13], null)).rejects.toMatchObject({
      status: 404,
      reason: "group_target_not_found",
    });
    await expect(undoOrganize("t1")).rejects.toMatchObject({
      status: 409,
      reason: "undo_conflict",
    });
    const expired = undoOrganize("t1");
    await expect(expired).rejects.toBeInstanceOf(RequestFailed);
    await expect(expired).rejects.toMatchObject({ status: 410, reason: "undo_expired" });
    expect(takeListSnapshot({ query: "" })).toBeDefined();
  });
});
