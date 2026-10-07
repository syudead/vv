import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { tagCountText, tagListView, type TagListViewInput } from "./tagListView";
import type { TagPageRows, TagRowsQuery } from "./tagPageRows";
import { moreIdle } from "./useTagPage";

function tag(id: number): Tag {
  return {
    id,
    name: `Tag ${String(id)}`,
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
  };
}

function page(
  rows: Tag[],
  query: Partial<TagRowsQuery> = {},
  overrides: Partial<TagPageRows> = {},
): TagPageRows {
  return {
    rows,
    total: rows.length,
    totalAll: rows.length,
    nextCursor: undefined,
    boundary: undefined,
    query: { query: "", tentativeOnly: false, unusedOnly: false, sort: "name", ...query },
    ...overrides,
  };
}

function view(input: Partial<TagListViewInput>) {
  const current = input.page;
  return tagListView({
    page: current,
    loadError: null,
    more: moreIdle,
    visibleCount: current?.rows.length ?? 0,
    creating: false,
    ...input,
  });
}

describe("tagListView", () => {
  it("先頭のページを受ける前は空の状態も表も出さず、どの操作もタグが無い扱い", () => {
    expect(view({ page: undefined })).toEqual({
      empty: null,
      showRows: false,
      staleList: false,
      tail: moreIdle,
      noTags: true,
    });
  });

  it("タグが 1 つも無ければ「タグが無い」で、作成中は表を出す", () => {
    expect(view({ page: page([]) })).toMatchObject({ empty: "tags", showRows: false });
    expect(view({ page: page([]), creating: true })).toMatchObject({
      empty: null,
      showRows: true,
    });
  });

  it("絞り込み中はタグが 0 でも「タグが無い」ではなく、絞り込みの空の状態にする", () => {
    expect(view({ page: page([], { unusedOnly: true }) }).empty).toBe("noUnused");
    expect(view({ page: page([], { tentativeOnly: true }) }).empty).toBe("noTentative");
  });

  it.each([
    [{ unusedOnly: true }, "noUnused"],
    [{ unusedOnly: true, query: "x" }, "noUnusedMatch"],
    [{ unusedOnly: true, tentativeOnly: true }, "noUnused"],
    [{ unusedOnly: true, tentativeOnly: true, query: "x" }, "noUnusedMatch"],
    [{ tentativeOnly: true }, "noTentative"],
    [{ tentativeOnly: true, query: "x" }, "noTentativeMatch"],
    [{ query: "x" }, "noMatch"],
  ] as const)(
    "合う行が無いときの空の状態は届いた行の条件で選ぶ（%o）",
    (query, empty) => {
      const current = page([], query, { totalAll: 5 });
      expect(view({ page: current })).toMatchObject({ empty, showRows: false });
    },
  );

  it("行が無くても続きがあれば、空の状態ではなく表と末尾の続きを出す", () => {
    const current = page([], {}, { total: 3, totalAll: 5, nextCursor: "c" });
    expect(view({ page: current })).toMatchObject({ empty: null, showRows: true });
  });

  it("差し込んで残した改名中の行があれば、読み込んだ行が無くても表を出す", () => {
    const current = page([], { query: "x" }, { totalAll: 5 });
    expect(view({ page: current, visibleCount: 1 })).toMatchObject({
      empty: null,
      showRows: true,
    });
  });

  it("一覧を持ったまま読めなければ「Stale list」で、末尾の続きは出さない", () => {
    const loading = { kind: "loading" } as const;
    expect(
      view({ page: page([tag(1)]), loadError: t.tags.gone, more: loading }),
    ).toMatchObject({ staleList: true, tail: moreIdle });
    expect(view({ page: undefined, loadError: t.tags.gone }).staleList).toBe(false);
    expect(view({ page: page([tag(1)]), more: loading }).tail).toBe(loading);
  });
});

describe("tagCountText", () => {
  it("絞っていなければ全部の数、検索・絞り込み中は「N of M」", () => {
    expect(tagCountText(undefined)).toBe(t.tags.loading);
    expect(tagCountText(page([tag(1), tag(2)]))).toBe(t.tags.count(2));
    const filtered = page([tag(1)], { tentativeOnly: true }, { totalAll: 9 });
    expect(tagCountText(filtered)).toBe(t.tags.filteredCount(1, 9));
    const searched = page([tag(1)], { query: "x" }, { totalAll: 9 });
    expect(tagCountText(searched)).toBe(t.tags.filteredCount(1, 9));
  });
});
