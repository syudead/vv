import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  bundleOnto,
  groupVideos,
  mergeGroups,
  moveCards,
  reorderGroup,
  undoOrganize,
} from "./organize";

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

  it("groupVideos は動画を画面の順に送り、from が null なら送らない。新しいグループの id を返し、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t1", groupId: 30 }))
      .mockResolvedValueOnce(json({ undoToken: "t2", groupId: 31 }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await expect(groupVideos([5, 3], 4, 7)).resolves.toEqual({
      undoToken: "t1",
      groupId: 30,
    });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    await expect(groupVideos([5], 4, null)).resolves.toEqual({
      undoToken: "t2",
      groupId: 31,
    });

    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/organize/group",
        method: "POST",
        body: {
          items: [
            { kind: "video", id: 5 },
            { kind: "video", id: 3 },
          ],
          target: 4,
          from: 7,
        },
      },
      {
        url: "/api/organize/group",
        method: "POST",
        body: { items: [{ kind: "video", id: 5 }], target: 4 },
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

  it("mergeGroups はカードを画面の順に into と送り、トークンを返し、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t3" }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await expect(
      mergeGroups(
        [
          { kind: "video", id: 5 },
          { kind: "group", id: 8 },
        ],
        9,
        null,
      ),
    ).resolves.toEqual({ undoToken: "t3" });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/organize/merge",
        method: "POST",
        body: {
          items: [
            { kind: "video", id: 5 },
            { kind: "group", id: 8 },
          ],
          into: 9,
        },
      },
    ]);
  });

  it("mergeGroups は from を渡すと開いているグループとして送る", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t4" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(mergeGroups([{ kind: "group", id: 8 }], 9, 1)).resolves.toEqual({
      undoToken: "t4",
    });
    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/organize/merge",
        method: "POST",
        body: { items: [{ kind: "group", id: 8 }], into: 9, from: 1 },
      },
    ]);
  });

  it("bundleOnto は動画と落とした先を送ってトークンを返し、一覧の控えを捨てる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ undoToken: "t1" }));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await expect(bundleOnto([9, 4], 5)).resolves.toEqual({ undoToken: "t1" });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/organize/bundle",
        method: "POST",
        body: { videoIds: [9, 4], target: 5 },
      },
    ]);
  });

  it("bundleOnto の 400 too_many_videos は limit 付きの RequestFailed になり、一覧の控えを残す", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValueOnce(
        json(
          {
            code: "invalid_request",
            reason: "too_many_videos",
            limit: 20,
            message: "too many",
          },
          400,
        ),
      ),
    );

    holdSnapshot();
    await expect(bundleOnto([9], 5)).rejects.toMatchObject({
      status: 400,
      reason: "too_many_videos",
      limit: 20,
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
