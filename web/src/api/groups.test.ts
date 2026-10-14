import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import {
  addToGroup,
  createGroup,
  dissolveGroup,
  listAllGroups,
  listGroups,
  moveGroupItem,
  removeFromGroup,
  renameGroup,
  tagGroup,
} from "./groups";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";

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

const group = {
  id: 7,
  name: "Mix",
  videoCount: 1,
  sizeBytes: 100,
  addedAt: "2026-10-11T00:00:00Z",
  previews: [],
  openVideoId: 1,
  videoIds: [1],
  tags: [],
};

/** holdSnapshot はライブラリの一覧の控えを 1 つ置く（動画ページへ離れたときと同じ）。 */
function holdSnapshot() {
  saveListSnapshot({ query: "" }, { items: [], total: 0, hasMore: false, scrollY: 0 });
}

describe("groups API", () => {
  it("listGroups は q・cursor・limit を付け、空の q は送らない", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ groups: [group], nextCursor: "c1" }))
      .mockResolvedValueOnce(json({ groups: [] }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listGroups({ q: " ", limit: 50 })).toEqual({
      groups: [group],
      nextCursor: "c1",
    });
    expect(await listGroups({ q: "Mi", cursor: "c1" })).toEqual({ groups: [] });
    expect(sent(fetchMock).map(({ url }) => url)).toEqual([
      "/api/groups?limit=50",
      "/api/groups?q=Mi&cursor=c1",
    ]);
  });

  it("createGroup・addToGroup は動画とグループを JSON で送り、404 を RequestFailed で返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(group, 201))
      .mockResolvedValueOnce(json(group))
      .mockResolvedValueOnce(
        json({ code: "not_found", reason: "group_target_not_found", message: "x" }, 404),
      );
    vi.stubGlobal("fetch", fetchMock);

    expect(await createGroup(" Mix ", { videoIds: [1], groupIds: [3] })).toEqual(group);
    expect(await addToGroup(7, { groupIds: [4] })).toEqual(group);
    const failure = await addToGroup(7, { videoIds: [9] }).catch(
      (error: unknown) => error,
    );

    expect(sent(fetchMock)).toEqual([
      {
        url: "/api/groups",
        method: "POST",
        body: { name: " Mix ", videoIds: [1], groupIds: [3] },
      },
      {
        url: "/api/groups/7/items",
        method: "POST",
        body: { videoIds: [], groupIds: [4] },
      },
      {
        url: "/api/groups/7/items",
        method: "POST",
        body: { videoIds: [9], groupIds: [] },
      },
    ]);
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).reason).toBe("group_target_not_found");
  });

  it("renameGroup・removeFromGroup・tagGroup は値を返し、dissolveGroup・moveGroupItem は 204 で終わる", async () => {
    const tagResult = { tag: { id: 2, name: "Mix" }, created: true };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ ...group, name: "Renamed" }))
      .mockResolvedValueOnce(json(group))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json(tagResult));
    vi.stubGlobal("fetch", fetchMock);

    expect((await renameGroup(7, "Renamed")).name).toBe("Renamed");
    expect(await removeFromGroup(7, [11, 12])).toEqual(group);
    await moveGroupItem(7, 11, 12);
    await moveGroupItem(7, 11, null);
    await dissolveGroup(7);
    expect(await tagGroup(7)).toEqual(tagResult);

    expect(sent(fetchMock)).toEqual([
      { url: "/api/groups/7", method: "PATCH", body: { name: "Renamed" } },
      { url: "/api/groups/7/items/remove", method: "POST", body: { itemIds: [11, 12] } },
      { url: "/api/groups/7/items/11/move", method: "POST", body: { after: 12 } },
      { url: "/api/groups/7/items/11/move", method: "POST", body: {} },
      { url: "/api/groups/7", method: "DELETE", body: undefined },
      { url: "/api/groups/7/tag", method: "POST", body: undefined },
      // タグ化の後は共有のタグの一覧を取り直す（タグが増えるか、本数が変わる）。
      { url: "/api/tags", method: "GET", body: undefined },
    ]);
  });

  it("dissolveGroup・moveGroupItem は失敗を RequestFailed で返す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ code: "not_found", message: "x" }, 404))
      .mockResolvedValueOnce(json({ code: "invalid_request", message: "x" }, 400));
    vi.stubGlobal("fetch", fetchMock);

    await expect(dissolveGroup(7)).rejects.toBeInstanceOf(RequestFailed);
    await expect(moveGroupItem(7, 11, 11)).rejects.toBeInstanceOf(RequestFailed);
  });

  it("改名・項目の削除・並べ替えの成功はライブラリの一覧の控えを捨て、失敗は残す", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ ...group, name: "Renamed" }))
      .mockResolvedValueOnce(json(group))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json({ code: "internal", message: "x" }, 500));
    vi.stubGlobal("fetch", fetchMock);

    holdSnapshot();
    await renameGroup(7, "Renamed");
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    holdSnapshot();
    await removeFromGroup(7, [11]);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    holdSnapshot();
    await moveGroupItem(7, 11, 12);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
    holdSnapshot();
    await expect(moveGroupItem(7, 11, 12)).rejects.toBeInstanceOf(RequestFailed);
    expect(takeListSnapshot({ query: "" })).toBeDefined();
  });

  it("listAllGroups はページの数で打ち切らず nextCursor が尽きるまで辿る", async () => {
    // 200 件のページを 101 回（2 万件を超える）。
    const pages = 101;
    const fetchMock = vi.fn<typeof fetch>((input) => {
      const cursor = new URL(String(input), "http://x").searchParams.get("cursor");
      const page = cursor === null ? 0 : Number(cursor);
      const next = page + 1 < pages ? { nextCursor: String(page + 1) } : {};
      return Promise.resolve(json({ groups: [{ ...group, id: page + 1 }], ...next }));
    });
    vi.stubGlobal("fetch", fetchMock);

    const groups = await listAllGroups();

    expect(groups).toHaveLength(pages);
    expect(groups.at(-1)?.id).toBe(pages);
  });

  it("listAllGroups は同じカーソルが二度返ったら欠けた一覧を返さずに失敗する", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json({ groups: [group], nextCursor: "loop" })),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listAllGroups()).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("createGroup・addToGroup は成功と 404 で一覧の控えを捨て、ほかの失敗では残す", async () => {
    const notFound = {
      code: "not_found",
      reason: "group_target_not_found",
      message: "x",
    };
    const cycle = { code: "conflict", reason: "group_cycle", message: "x" };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(group, 201))
      .mockResolvedValueOnce(json(group))
      .mockResolvedValueOnce(json(notFound, 404))
      .mockResolvedValueOnce(json(cycle, 409));
    vi.stubGlobal("fetch", fetchMock);
    const snapshot = () => takeListSnapshot({ query: "" });

    holdSnapshot();
    await createGroup("Mix", { videoIds: [1] });
    expect(snapshot()).toBeUndefined();

    holdSnapshot();
    await addToGroup(7, { videoIds: [1] });
    expect(snapshot()).toBeUndefined();

    holdSnapshot();
    await addToGroup(7, { videoIds: [9] }).catch(() => undefined);
    expect(snapshot()).toBeUndefined();

    holdSnapshot();
    await addToGroup(7, { groupIds: [3] }).catch(() => undefined);
    expect(snapshot()).toBeDefined();
  });
});
