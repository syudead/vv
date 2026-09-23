import { beforeEach, describe, expect, it } from "vitest";

import type { Video } from "./client";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";

/**
 * 一覧の復元状態。
 *
 * 要点は 2 つある。**鍵が違えば取れない**こと（別の絞り込みの一覧を戻り先に
 * してはならない）と、**直近の 1 件しか持たない**ことである。後者を外すと、
 * 検索語を変えて回っただけで数千件の Video がメモリに残る。
 */

/** item は控えに入れる 1 件を作る。 */
function item(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
  };
}

/** body は鍵以外の中身を作る。 */
function body(ids: number[], scrollY = 0) {
  return {
    items: ids.map(item),
    total: ids.length,
    cursor: "cursor-1",
    hasMore: true,
    scrollY,
  };
}

beforeEach(() => {
  clearListSnapshot();
});

describe("ListSnapshot（一覧の復元状態）", () => {
  it("保存した鍵と同じ鍵で取れる", () => {
    saveListSnapshot({ query: "ねこ", sort: "titleAsc" }, body([1, 2], 640));

    const restored = takeListSnapshot({ query: "ねこ", sort: "titleAsc" });
    expect(restored?.items.map((video) => video.id)).toEqual([1, 2]);
    expect(restored?.total).toBe(2);
    expect(restored?.cursor).toBe("cursor-1");
    expect(restored?.hasMore).toBe(true);
    expect(restored?.scrollY).toBe(640);
  });

  it("鍵が違えば取れない", () => {
    saveListSnapshot({ query: "ねこ", sort: "titleAsc" }, body([1, 2]));

    // 検索語が違う。別の絞り込みの一覧を戻り先にしてはならない。
    expect(takeListSnapshot({ query: "いぬ", sort: "titleAsc" })).toBeUndefined();
    // 並び順が違う。並びが違えば同じ項目でも順序が違う。
    expect(takeListSnapshot({ query: "ねこ", sort: "addedDesc" })).toBeUndefined();
  });

  it("clearListSnapshot のあとは取れない", () => {
    saveListSnapshot({ query: "", sort: "addedDesc" }, body([1]));
    expect(takeListSnapshot({ query: "" })).toBeDefined();

    // 取り込みが終わって読み直すとき、取り込む前の一覧に戻してはならない。
    clearListSnapshot();
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
  });

  it("2 件目を保存すると 1 件目は捨てられる", () => {
    saveListSnapshot({ query: "ねこ" }, body([1, 2]));
    saveListSnapshot({ query: "いぬ" }, body([3]));

    expect(takeListSnapshot({ query: "いぬ" })?.items.map((video) => video.id)).toEqual([
      3,
    ]);
    // 鍵ごとに溜めない。持つのは直近の 1 件だけである。
    expect(takeListSnapshot({ query: "ねこ" })).toBeUndefined();
  });

  it("鍵の正規化が検索語の前後の空白と並び順の既定値を吸収する", () => {
    saveListSnapshot({ query: "  ねこ  ", sort: "addedDesc" }, body([1]));

    // 空白の有無で戻り先を失わない。
    expect(takeListSnapshot({ query: "ねこ", sort: "addedDesc" })).toBeDefined();
    // `/` と `/?sort=addedDesc` は利用者から見て同じ一覧である。
    expect(takeListSnapshot({ query: "ねこ" })).toBeDefined();
    // 既定と違う並び順まで吸収してはならない。
    expect(takeListSnapshot({ query: "ねこ", sort: "titleAsc" })).toBeUndefined();
  });
});
