import { afterEach, describe, expect, it, vi } from "vitest";

import {
  addTagSynonym,
  createTag,
  currentTags,
  deleteTag,
  getTags,
  mergeTag,
  refreshTags,
  removeTagSynonym,
  renameTag,
  subscribeTags,
} from "./tags";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const tag = (overrides: Partial<{ id: number; name: string }> = {}) => ({
  id: overrides.id ?? 1,
  name: overrides.name ?? "旅行",
  synonyms: [] as string[],
  videoCount: 0,
});

const key = { query: "", sort: "addedDesc" as const };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("共有のタグの一覧の保持", () => {
  it("同時に呼んでも要求は1つにまとめ、以後はcurrentTagsで同期に読める", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 1 })] }));
    vi.stubGlobal("fetch", fetch);

    const [a, b] = await Promise.all([getTags(), getTags()]);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(a).toEqual(b);
    expect(currentTags()).toEqual(a);

    // 一度取れていれば、以後のgetTagsは取り直さない。
    await getTags();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("refreshTagsは常に取り直し、購読者へ知らせる", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof globalThis.fetch>()
        .mockResolvedValueOnce(
          jsonResponse({ items: [tag({ id: 1 }), tag({ id: 2, name: "料理" })] }),
        ),
    );

    const heard: number[] = [];
    const unsubscribe = subscribeTags((tags) => heard.push(tags.length));
    const refreshed = await refreshTags();
    unsubscribe();

    expect(refreshed).toHaveLength(2);
    expect(heard).toEqual([2]);
    expect(currentTags()).toHaveLength(2);
  });
});

describe("タグの変更が成功した後の後始末", () => {
  function stubMutation(response: Response, refreshedItems: unknown[]) {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(response)
      .mockResolvedValueOnce(jsonResponse({ items: refreshedItems }));
    vi.stubGlobal("fetch", fetch);
    return fetch;
  }

  it("createTagは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    stubMutation(jsonResponse(tag({ id: 9, name: "新しいタグ" })), [
      tag({ id: 9, name: "新しいタグ" }),
    ]);

    const created = await createTag("新しいタグ");
    expect(created.name).toBe("新しいタグ");
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 9, name: "新しいタグ" })]);
    });
  });

  it("renameTagは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    stubMutation(jsonResponse(tag({ id: 1, name: "改名後" })), [
      tag({ id: 1, name: "改名後" }),
    ]);

    await renameTag(1, "改名後");
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()?.[0]?.name).toBe("改名後");
    });
  });

  it("deleteTagは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    stubMutation(new Response(null, { status: 204 }), []);

    await deleteTag(1);
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([]);
    });
  });

  it("mergeTagは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const fetch = stubMutation(jsonResponse(tag({ id: 1 })), [tag({ id: 1 })]);

    await mergeTag(1, 2);
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/tags/1/merge",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ sourceId: 2 }) }),
    );
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 1 })]);
    });
  });

  it("addTagSynonymは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const fetch = stubMutation(jsonResponse({ ...tag({ id: 1 }), synonyms: ["旧名"] }), [
      { ...tag({ id: 1 }), synonyms: ["旧名"] },
    ]);

    await addTagSynonym(1, "旧名", 7);
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/tags/1/synonyms",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ name: "旧名", mergeTagId: 7 }),
      }),
    );
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()?.[0]?.synonyms).toEqual(["旧名"]);
    });
  });

  it("removeTagSynonymは成功後に共有の一覧を取り直し、一覧の控えを捨てる", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const fetch = stubMutation(new Response(null, { status: 204 }), [tag({ id: 1 })]);

    await removeTagSynonym(1, "旧名");
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/tags/1/synonyms?name=%E6%97%A7%E5%90%8D",
      {
        method: "DELETE",
        signal: undefined,
      },
    );
    expect(takeListSnapshot(key)).toBeUndefined();
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 1 })]);
    });
  });

  it("失敗した変更は一覧の控えを残し、共有の一覧も取り直さない", async () => {
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: "tag_name_taken", message: "x" }, 409));
    vi.stubGlobal("fetch", fetch);

    await expect(createTag("重複")).rejects.toThrow();
    expect(takeListSnapshot(key)).toBeDefined();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
