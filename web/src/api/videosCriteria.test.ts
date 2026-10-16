import { describe, expect, it } from "vitest";

import { listsSmartFolders, type VideosCriteria } from "./videosCriteria";

// ライブラリの一番上にスマートフォルダのカードが並ぶ条件（specs/052-smart-folders/research.md R-7）。

describe("listsSmartFolders", () => {
  const base: VideosCriteria = { sort: "titleAsc" };

  it("並べ替え、空白だけの検索語、すべての視聴状態、お気に入りのみでは並ぶ", () => {
    expect(listsSmartFolders(base)).toBe(true);
    expect(listsSmartFolders({ ...base, query: "  ", watch: "all" })).toBe(true);
    expect(listsSmartFolders({ ...base, favorite: true, tag: [] })).toBe(true);
  });

  it.each<[string, Partial<VideosCriteria>]>([
    ["検索語", { query: "旅" }],
    ["視聴状態", { watch: "unwatched" }],
    ["再生できるもの", { playable: true }],
    ["タグ", { tag: [1] }],
    ["人物", { person: 2 }],
    ["団体", { organization: 3 }],
    ["タグの種類", { tagKind: 4 }],
    ["追加日", { addedWithin: 7 }],
    ["長さの下限", { minDuration: 60 }],
    ["長さの上限", { maxDuration: 299 }],
  ])("%sで絞り込むと並ばない", (_name, filter) => {
    expect(listsSmartFolders({ ...base, ...filter })).toBe(false);
  });
});
