import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { tagCountText } from "./tagListView";
import type { TagPageRows, TagRowsQuery } from "./tagPageRows";

function tag(id: number): Tag {
  return {
    kind: null,
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

describe("tagCountText", () => {
  it("絞っていなければ全部の数、検索・絞り込み中は「N of M」", () => {
    expect(tagCountText(undefined)).toBe(t.tags.loading);
    expect(tagCountText(page([tag(1), tag(2)]))).toBe(t.tags.count(2));
    const filtered = page([tag(1)], { tentativeOnly: true }, { totalAll: 9 });
    expect(tagCountText(filtered)).toBe(t.tags.filteredCount(1, 9));
    const searched = page([tag(1)], { query: "x" }, { totalAll: 9 });
    expect(tagCountText(searched)).toBe(t.tags.filteredCount(1, 9));
    const byKind = page([tag(1)], { kind: "none" }, { totalAll: 9 });
    expect(tagCountText(byKind)).toBe(t.tags.filteredCount(1, 9));
  });
});
