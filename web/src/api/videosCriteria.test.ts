import { describe, expect, it } from "vitest";

import { listsSmartFolders, type VideosCriteria } from "./videosCriteria";

describe("listsSmartFolders", () => {
  const base: VideosCriteria = { sort: "titleAsc" };

  it("並べ替え、空白だけの検索語、お気に入りのみでは並ぶ", () => {
    expect(listsSmartFolders(base)).toBe(true);
    expect(listsSmartFolders({ ...base, query: "  " })).toBe(true);
    expect(listsSmartFolders({ ...base, favorite: true, tag: [] })).toBe(true);
  });

  it.each<[string, Partial<VideosCriteria>]>([
    ["検索語", { query: "旅" }],
    ["タグ", { tag: [1] }],
    ["人物", { person: 2 }],
    ["団体", { organization: 3 }],
    ["タグの種類", { tagKind: 4 }],
  ])("%sで絞り込むと並ばない", (_name, filter) => {
    expect(listsSmartFolders({ ...base, ...filter })).toBe(false);
  });
});
