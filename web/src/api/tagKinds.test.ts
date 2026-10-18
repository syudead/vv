import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  __resetTagKindsForTest,
  createTagKind,
  currentTagKinds,
  deleteTagKind,
  getTagKinds,
  refreshTagKinds,
  renameTagKind,
  sortTagKinds,
  subscribeTagKinds,
} from "./tagKinds";
import { __resetTagsForTest } from "./tags";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const kind = (id: number, name: string, tagCount = 0) => ({
  id,
  name,
  tagCount,
  createdAt: "2026-01-01T00:00:00Z",
});

const presets = [
  kind(1, "Genre"),
  kind(2, "Work"),
  kind(3, "Place"),
  kind(4, "Character"),
];

function urlOf(input: Parameters<typeof globalThis.fetch>[0]): string {
  if (typeof input === "string") return input;
  return input instanceof URL ? input.pathname : new URL(input.url).pathname;
}

function routes(handlers: Record<string, () => Response>) {
  const calls: string[] = [];
  const fetch = vi.fn<typeof globalThis.fetch>((input, init) => {
    const key = `${init?.method ?? "GET"} ${urlOf(input)}`;
    calls.push(key);
    const handler = handlers[key];
    return Promise.resolve(handler === undefined ? jsonResponse({}, 500) : handler());
  });
  vi.stubGlobal("fetch", fetch);
  return calls;
}

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  __resetTagKindsForTest();
  __resetTagsForTest();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("sortTagKinds", () => {
  it("名前の自然順に並べ、大文字小文字だけが違う名前は id の順にする", () => {
    const sorted = sortTagKinds([
      kind(4, "era"),
      kind(2, "Item 10"),
      kind(3, "Era"),
      kind(1, "Item 9"),
    ]);
    expect(sorted.map((k) => k.id)).toEqual([3, 4, 1, 2]);
  });
});

describe("共有の種類の一覧の保持", () => {
  it("同時に呼んでも要求は 1 つにまとめ、以後は currentTagKinds で同期に読める", async () => {
    const calls = routes({
      "GET /api/tag-kinds": () => jsonResponse({ items: presets }),
    });

    const [a, b] = await Promise.all([getTagKinds(), getTagKinds()]);
    expect(calls).toEqual(["GET /api/tag-kinds"]);
    expect(a.map((k) => k.name)).toEqual(["Genre", "Work", "Place", "Character"]);
    expect(b).toEqual(a);
    expect(currentTagKinds()).toEqual(a);

    await getTagKinds();
    expect(calls).toHaveLength(1);
  });

  it("失敗した取得のあとの getTagKinds は取り直す", async () => {
    let fail = true;
    routes({
      "GET /api/tag-kinds": () =>
        fail
          ? jsonResponse({ code: "internal", message: "x" }, 500)
          : jsonResponse({ items: presets }),
    });
    await expect(getTagKinds()).rejects.toBeInstanceOf(RequestFailed);
    fail = false;
    await expect(getTagKinds()).resolves.toHaveLength(4);
  });

  it("作ったあとは保持を捨て、次の getTagKinds が取り直す", async () => {
    let items = presets;
    const calls = routes({
      "GET /api/tag-kinds": () => jsonResponse({ items }),
      "POST /api/tag-kinds": () => jsonResponse(kind(5, "Era"), 201),
    });
    await getTagKinds();

    const created = await createTagKind("Era");
    expect(created.name).toBe("Era");
    expect(currentTagKinds()).toBeUndefined();

    items = [...presets, kind(5, "Era")];
    expect(await getTagKinds()).toHaveLength(5);
    expect(calls).toEqual([
      "GET /api/tag-kinds",
      "POST /api/tag-kinds",
      "GET /api/tag-kinds",
    ]);
  });

  it("変更の前に始まった取得の結果は保持にも呼び手にも返さない", async () => {
    let items = presets;
    let release: (() => void) | undefined;
    routes({
      "GET /api/tag-kinds": () => jsonResponse({ items }),
      "POST /api/tag-kinds": () => jsonResponse(kind(5, "Era"), 201),
    });
    const original = globalThis.fetch;
    let first = true;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof globalThis.fetch>(async (input, init) => {
        if (first && init?.method === undefined) {
          first = false;
          const response = await original(input, init);
          await new Promise<void>((resolve) => {
            release = resolve;
          });
          return response;
        }
        return original(input, init);
      }),
    );

    const stale = getTagKinds();
    await flush();
    await createTagKind("Era");
    items = [...presets, kind(5, "Era")];
    release?.();

    expect(await stale).toHaveLength(5);
    expect(currentTagKinds()).toHaveLength(5);
  });

  it("古い取得が失敗しても、あとから始めた取得を共有し続ける", async () => {
    const pending: ((response: Response) => void)[] = [];
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof globalThis.fetch>((input) => {
        calls.push(urlOf(input));
        return new Promise<Response>((resolve) => pending.push(resolve));
      }),
    );

    const stale = getTagKinds();
    const fresh = refreshTagKinds();
    await flush();
    pending[0]!(jsonResponse({ code: "internal", message: "x" }, 500));
    await expect(stale).rejects.toBeInstanceOf(RequestFailed);

    const shared = getTagKinds();
    await flush();
    expect(calls).toEqual(["/api/tag-kinds", "/api/tag-kinds"]);
    pending[1]!(jsonResponse({ items: presets }));
    expect(await shared).toHaveLength(4);
    expect(await fresh).toHaveLength(4);
  });

  it("変更のあとの取り直しが失敗しても、変更そのものは成功として返す", async () => {
    const calls = routes({
      "POST /api/tag-kinds": () => jsonResponse(kind(5, "Era"), 201),
      "PATCH /api/tag-kinds/5": () => jsonResponse(kind(5, "Period")),
    });
    const unsubscribe = subscribeTagKinds(() => undefined);
    // Missing GET routes fail with 500; mutation completion does not await refresh.
    await expect(createTagKind("Era")).resolves.toMatchObject({ name: "Era" });
    await expect(renameTagKind(5, "Period")).resolves.toMatchObject({ name: "Period" });
    await flush();
    unsubscribe();
    expect(calls).toEqual([
      "POST /api/tag-kinds",
      "GET /api/tag-kinds",
      "PATCH /api/tag-kinds/5",
      "GET /api/tag-kinds",
      "GET /api/tags",
    ]);
  });

  it("購読者がいれば種類を変えたあとに取り直して知らせる", async () => {
    routes({
      "GET /api/tag-kinds": () => jsonResponse({ items: presets }),
      "POST /api/tag-kinds": () => jsonResponse(kind(5, "Era"), 201),
    });
    const heard: number[] = [];
    const unsubscribe = subscribeTagKinds((kinds) => heard.push(kinds.length));
    await createTagKind("Era");
    await flush();
    unsubscribe();
    expect(heard).toEqual([4]);
  });
});

describe("名前の変更と削除", () => {
  it("名前を変えたあとは共有のタグの一覧を取り直し、動画一覧の控えを捨てる", async () => {
    const calls = routes({
      "PATCH /api/tag-kinds/1": () => jsonResponse(kind(1, "Style", 2)),
      "GET /api/tags": () => jsonResponse({ items: [], total: 0, totalAll: 0 }),
    });
    const key = { query: "", sort: "addedDesc" as const };
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });

    const renamed = await renameTagKind(1, "Style");
    await flush();

    expect(renamed.name).toBe("Style");
    expect(calls).toEqual(["PATCH /api/tag-kinds/1", "GET /api/tags"]);
    expect(takeListSnapshot(key)).toBeUndefined();
  });

  it("消したあとも共有のタグの一覧を取り直す", async () => {
    const calls = routes({
      "DELETE /api/tag-kinds/2": () => new Response(null, { status: 204 }),
      "GET /api/tags": () => jsonResponse({ items: [], total: 0, totalAll: 0 }),
    });
    await deleteTagKind(2);
    await flush();
    expect(calls).toEqual(["DELETE /api/tag-kinds/2", "GET /api/tags"]);
  });

  it("tag_kind_not_found は投げ直し、保持を捨て、タグの一覧と動画一覧の控えも新しくする", async () => {
    const calls = routes({
      "GET /api/tag-kinds": () => jsonResponse({ items: presets }),
      "GET /api/tags": () => jsonResponse({ items: [], total: 0, totalAll: 0 }),
      "DELETE /api/tag-kinds/9": () =>
        jsonResponse(
          {
            code: "not_found",
            reason: "tag_kind_not_found",
            message: "Tag kind not found.",
          },
          404,
        ),
    });
    await getTagKinds();
    const key = { query: "", sort: "addedDesc" as const };
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });

    const error = await deleteTagKind(9).catch((caught: unknown) => caught);
    await flush();
    expect(error).toBeInstanceOf(RequestFailed);
    expect((error as RequestFailed).reason).toBe("tag_kind_not_found");
    expect(currentTagKinds()).toBeUndefined();
    expect(calls).toEqual([
      "GET /api/tag-kinds",
      "DELETE /api/tag-kinds/9",
      "GET /api/tags",
    ]);
    expect(takeListSnapshot(key)).toBeUndefined();
  });
});
