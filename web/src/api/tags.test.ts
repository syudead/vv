import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  __resetTagsForTest,
  addTagSynonym,
  attachVideoTagByID,
  attachVideoTagByName,
  createTag,
  currentTags,
  deleteTag,
  detachVideoTag,
  getTags,
  mergeTag,
  refreshTags,
  removeTagSynonym,
  renameTag,
  subscribeTags,
  summarizeVideoTags,
} from "./tags";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const tag = (
  overrides: Partial<{
    id: number;
    name: string;
    synonyms: string[];
    videoCount: number;
  }> = {},
) => ({
  id: overrides.id ?? 1,
  name: overrides.name ?? "旅行",
  synonyms: overrides.synonyms ?? ([] as string[]),
  videoCount: overrides.videoCount ?? 0,
});

const key = { query: "", sort: "addedDesc" as const };

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  // モジュールの共有の保持（held・generation・pendingGet・listeners）は
  // テストをまたいで残るので、各テストが「まだ何も取得していない」状態から
  // 始められるように明示的にリセットする（N5）。
  __resetTagsForTest();
});

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

  it("共有のGETはどの呼び出し元のAbortSignalも使わない（B2）", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 1 })] }));
    vi.stubGlobal("fetch", fetch);

    await getTags();

    // getTags/refreshTagsはAbortSignalを受け取るパラメータを持たない。共有の
    // 取得へfetchへ渡るinitにsignalが乗らないことを確かめる。
    expect(fetch).toHaveBeenCalledWith("/api/tags", undefined);
  });

  it("1人のcallerが自分の待ちを諦めても、他の待ち手はそのまま結果を受け取れる（B2）", async () => {
    let resolveGet: (response: Response) => void = () => undefined;
    const fetch = vi.fn<typeof globalThis.fetch>().mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          resolveGet = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetch);

    const controllerA = new AbortController();
    const a = getTags();
    const b = getTags();

    // 呼び出し元Aが（自分のUIの都合などで）持っているAbortControllerを中断させても、
    // 共有の取得にはそのAbortSignalが渡っていないので、影響しない。
    controllerA.abort();
    resolveGet(jsonResponse({ items: [tag({ id: 1 })] }));

    await expect(a).resolves.toEqual([tag({ id: 1 })]);
    await expect(b).resolves.toEqual([tag({ id: 1 })]);
  });

  it("変更で始まった取り直しは、始まっていた進行中のGETに乗らず、新しい要求を送る（B1）", async () => {
    let resolveOldGet: (response: Response) => void = () => undefined;
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      // 1回目: getTags()が始めるGET。しばらく応答が来ない。
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveOldGet = resolve;
          }),
      )
      // 2回目: renameTagのPATCH。
      .mockResolvedValueOnce(jsonResponse(tag({ id: 1, name: "改名後" })))
      // 3回目: 改名の成功を受けて afterTagChanged が始める取り直し。
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 1, name: "改名後" })] }));
    vi.stubGlobal("fetch", fetch);

    const staleGet = getTags();
    await renameTag(1, "改名後");

    // 改名後の取り直し（3回目）はすでに終わっているはずで、held は最新に
    // 差し替わっている。
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 1, name: "改名後" })]);
    });
    expect(fetch).toHaveBeenCalledTimes(3);

    // 先に始まっていた古いGET（1回目）が、あとから古い中身で応答しても、
    // 新しい方の結果を上書きしない。
    resolveOldGet(jsonResponse({ items: [tag({ id: 1, name: "旧名" })] }));
    await staleGet;
    await flush();
    expect(currentTags()).toEqual([tag({ id: 1, name: "改名後" })]);
  });

  // Devin の指摘: 追い越された取得は、`held` がまだ一度も埋まっていなくても
  // （＝ `held ?? tags` の `tags` が漏れる状況でも）、自分のまだ何も反映して
  // いない応答をそのまま返してはならない。勝った（今の generation を持つ）
  // 取得の結果を待って、それに落ち着く。
  it("heldが空のうちに追い越された取得は、勝った取得の結果に落ち着く（Devinの指摘）", async () => {
    let resolveFirst: (response: Response) => void = () => undefined;
    let resolveSecond: (response: Response) => void = () => undefined;
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      // 1回目: getTags() が始める GET。まだ何も返っていない（held は空のまま）。
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      // 2回目: refreshTags() が始める、追い越す側の GET。
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveSecond = resolve;
          }),
      );
    vi.stubGlobal("fetch", fetch);

    const first = getTags();
    const second = refreshTags();

    // 1回目（追い越された、まだ何も反映していない）の応答が、2回目より先に
    // 届く。held はまだ一度も埋まっていない。
    resolveFirst(jsonResponse({ items: [] }));
    await flush();
    // 1回目の（空の）中身をそのまま held へ反映してはいない。
    expect(currentTags()).toBeUndefined();

    // 2回目（勝った、今の generation を持つ）の応答が届く。
    resolveSecond(jsonResponse({ items: [tag({ id: 1, name: "後から作った" })] }));

    await expect(second).resolves.toEqual([tag({ id: 1, name: "後から作った" })]);
    // 追い越された1回目も、自分の（空の）応答ではなく、勝った2回目の結果に
    // 落ち着く。
    await expect(first).resolves.toEqual([tag({ id: 1, name: "後から作った" })]);
    expect(currentTags()).toEqual([tag({ id: 1, name: "後から作った" })]);
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
    // 作ったタグは同じ名前のフォルダの下の既存の動画にもすぐ付くので
    // （017 の data-model.md §4）、控えを復元すると古いタグが戻ってしまう。
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
    expect(currentTags()).toBeUndefined();
  });

  it("変更成功後の取り直しに失敗しても、変更自体は成功として扱われ、未処理のrejectを残さない（B2）", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse(tag({ id: 1, name: "改名後" })))
      .mockRejectedValueOnce(new Error("network down"));
    vi.stubGlobal("fetch", fetch);

    await expect(renameTag(1, "改名後")).resolves.toMatchObject({ name: "改名後" });
    // 取り直しは失敗しているはずだが、そのために未処理の reject が残っていれば
    // Vitest がこのテストを失敗させる。ここまで到達できれば揉み消せている。
    await flush();
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});

describe("古いタグを使った操作の後始末（S3、Issue #266 項目5）", () => {
  it("tag_not_foundを受けたら共有の一覧を取り直してから投げ直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: "tag_not_found", message: "x" }, 404))
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 2, name: "別のタグ" })] }));
    vi.stubGlobal("fetch", fetch);

    await expect(renameTag(999, "新しい名前")).rejects.toMatchObject({
      code: "tag_not_found",
    });
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 2, name: "別のタグ" })]);
    });
  });

  it("tag_merge_requiredを受けたら共有の一覧を取り直してから投げ直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(
        jsonResponse({ code: "tag_merge_required", message: "x" }, 409),
      )
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 3, name: "旧名" })] }));
    vi.stubGlobal("fetch", fetch);

    await expect(addTagSynonym(1, "旧名")).rejects.toMatchObject({
      code: "tag_merge_required",
    });
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 3, name: "旧名" })]);
    });
  });

  it("tag_name_takenのような無関係な誤りでは共有の一覧を取り直さない", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: "tag_name_taken", message: "x" }, 409));
    vi.stubGlobal("fetch", fetch);

    await expect(renameTag(1, "重複")).rejects.toMatchObject({ code: "tag_name_taken" });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(currentTags()).toBeUndefined();
  });
});

describe("動画へのタグの付け外し・要約（issue 267）", () => {
  it("attachVideoTagByIDはPOST /api/video-tagsをid指定で送り、付け外しの通知を出す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 2, name: "旅行" }, applied: 2 }));
    vi.stubGlobal("fetch", fetch);
    const { subscribeVideoTags } = await import("./videoTagsEvents");
    const notified: unknown[] = [];
    const unsubscribe = subscribeVideoTags((videoIds, tag, action) => {
      notified.push({ videoIds, tag, action });
    });

    const result = await attachVideoTagByID([1, 2], 2);

    expect(result).toEqual({ tag: { id: 2, name: "旅行" }, applied: 2 });
    const [, init] = fetch.mock.calls[0] ?? [];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      videoIds: [1, 2],
      action: "add",
      tag: { id: 2 },
    });
    expect(notified).toEqual([
      { videoIds: [1, 2], tag: { id: 2, name: "旅行" }, action: "add" },
    ]);
    unsubscribe();
  });

  // (5): 本数（videoCount）が変わるので、id 指定の付与でも共有の一覧を
  // 取り直し、候補の本数を新しく保つ。
  it("attachVideoTagByIDも共有のタグの一覧を取り直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 2, name: "旅行" }, applied: 1 }))
      .mockResolvedValueOnce(
        jsonResponse({ items: [tag({ id: 2, name: "旅行", videoCount: 1 })] }),
      );
    vi.stubGlobal("fetch", fetch);

    await attachVideoTagByID([1], 2);
    await flush();

    expect(fetch.mock.calls[1]?.[0]).toBe("/api/tags");
    expect(currentTags()).toEqual([tag({ id: 2, name: "旅行", videoCount: 1 })]);
  });

  it("attachVideoTagByNameはtag.nameで送る", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 9, name: "新規" }, applied: 1 }));
    vi.stubGlobal("fetch", fetch);

    await attachVideoTagByName([1], "新規");

    const [, init] = fetch.mock.calls[0] ?? [];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      videoIds: [1],
      action: "add",
      tag: { name: "新規" },
    });
  });

  // issue 268 の受け入れ条件1: 名前で付けたタグが新しく作られたかもしれないので、
  // createTag と同じく共有の一覧を取り直す（別の動画の再生画面の候補にも出る）。
  it("attachVideoTagByNameは共有のタグの一覧を取り直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 9, name: "新規" }, applied: 1 }))
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 9, name: "新規" })] }));
    vi.stubGlobal("fetch", fetch);

    await attachVideoTagByName([1], "新規");
    await flush();

    expect(fetch.mock.calls[1]?.[0]).toBe("/api/tags");
    expect(currentTags()).toEqual([tag({ id: 9, name: "新規" })]);
  });

  // 名前で付けて新しいタグが作られたら、フォルダ名から既存の動画にも付くので
  // 一覧の控えを捨てる（017 の data-model.md §4）。既存のタグなら控えは残す。
  it("attachVideoTagByNameは新しく作られたタグなら一覧の控えを捨てる", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 2, name: "旅行" })] }))
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 9, name: "新規" }, applied: 1 }))
      .mockResolvedValueOnce(
        jsonResponse({
          items: [tag({ id: 2, name: "旅行" }), tag({ id: 9, name: "新規" })],
        }),
      );
    vi.stubGlobal("fetch", fetch);
    await refreshTags();
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });

    await attachVideoTagByName([1], "新規");
    await flush();

    expect(takeListSnapshot(key)).toBeUndefined();
  });

  it("attachVideoTagByNameは既存のタグなら一覧の控えを残す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 2, name: "旅行" })] }))
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 2, name: "旅行" }, applied: 1 }))
      .mockResolvedValueOnce(
        jsonResponse({ items: [tag({ id: 2, name: "旅行", videoCount: 1 })] }),
      );
    vi.stubGlobal("fetch", fetch);
    await refreshTags();
    saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });

    await attachVideoTagByName([1], "旅行");
    await flush();

    expect(takeListSnapshot(key)).toBeDefined();
    expect(fetch.mock.calls[2]?.[0]).toBe("/api/tags");
  });

  it("detachVideoTagはaction=removeとid指定で送り、除去の通知を出す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 2, name: "旅行" }, applied: 1 }));
    vi.stubGlobal("fetch", fetch);
    const { subscribeVideoTags } = await import("./videoTagsEvents");
    const notified: unknown[] = [];
    const unsubscribe = subscribeVideoTags((videoIds, tag, action) => {
      notified.push({ videoIds, tag, action });
    });

    await detachVideoTag([2], 2);

    const [, init] = fetch.mock.calls[0] ?? [];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      videoIds: [2],
      action: "remove",
      tag: { id: 2 },
    });
    expect(notified).toEqual([
      { videoIds: [2], tag: { id: 2, name: "旅行" }, action: "remove" },
    ]);
    unsubscribe();
  });

  // (5): detachVideoTag も本数が変わるので、共有の一覧を取り直す。
  it("detachVideoTagも共有のタグの一覧を取り直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ tag: { id: 2, name: "旅行" }, applied: 1 }))
      .mockResolvedValueOnce(
        jsonResponse({ items: [tag({ id: 2, name: "旅行", videoCount: 0 })] }),
      );
    vi.stubGlobal("fetch", fetch);

    await detachVideoTag([2], 2);
    await flush();

    expect(fetch.mock.calls[1]?.[0]).toBe("/api/tags");
    expect(currentTags()).toEqual([tag({ id: 2, name: "旅行", videoCount: 0 })]);
  });

  it("tag_not_foundを受けたら共有の一覧を取り直してから投げ直す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: "tag_not_found", message: "x" }, 404))
      .mockResolvedValueOnce(jsonResponse({ items: [tag({ id: 4, name: "現存" })] }));
    vi.stubGlobal("fetch", fetch);

    await expect(attachVideoTagByID([1], 999)).rejects.toMatchObject({
      code: "tag_not_found",
    });
    await vi.waitFor(() => {
      expect(currentTags()).toEqual([tag({ id: 4, name: "現存" })]);
    });
  });

  it("summarizeVideoTagsはtotal・itemsを返す", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(
      jsonResponse({
        total: 3,
        items: [{ tag: { id: 1, name: "旅行" }, count: 2 }],
      }),
    );
    vi.stubGlobal("fetch", fetch);

    const result = await summarizeVideoTags([1, 2, 3]);

    expect(result).toEqual({
      total: 3,
      items: [{ tag: { id: 1, name: "旅行" }, count: 2 }],
    });
    const [, init] = fetch.mock.calls[0] ?? [];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      videoIds: [1, 2, 3],
    });
  });
});
