import { describe, expect, it } from "vitest";

import type { SmartFolderSummary } from "../api/smartFolders";
import {
  descendantIds,
  folderSort,
  hasRuleMember,
  parentChoices,
  ruleFromLibrary,
  ruleKey,
  validRule,
} from "./smartRule";

// スマートフォルダの条件と親の木。

const unfiltered = {
  query: "",
  watch: "all" as const,
  playable: false,
  favorite: false,
  sort: "addedDesc" as const,
};

describe("ruleFromLibrary", () => {
  it("ライブラリの絞り込みの効いている条件だけを持つ", () => {
    expect(
      ruleFromLibrary({
        criteria: { ...unfiltered, watch: "unwatched" },
        tagIds: [3],
        credits: {},
        addedLength: {},
      }),
    ).toEqual({ watch: "unwatched", tags: [3] });
    expect(
      ruleFromLibrary({
        criteria: { ...unfiltered, query: "kyoto", favorite: true, playable: true },
        tagIds: [],
        credits: { person: 4, organization: 5, tagKind: 6 },
        addedLength: { addedWithin: 7, minDuration: 300, maxDuration: 1199 },
      }),
    ).toEqual({
      q: "kyoto",
      favorite: true,
      playable: true,
      person: 4,
      organization: 5,
      tagKind: 6,
      addedWithin: 7,
      minDuration: 300,
      maxDuration: 1199,
    });
  });
});

describe("条件の検査", () => {
  it("空の条件は条件として数えない", () => {
    expect(hasRuleMember({})).toBe(false);
    expect(hasRuleMember({ q: "", tags: [], favorite: false })).toBe(false);
    expect(hasRuleMember({ playable: true })).toBe(true);
  });

  it("サーバーの範囲の外の条件は送れない", () => {
    expect(validRule({ tags: [1, 2] })).toBe(true);
    expect(validRule({ q: "a".repeat(501) })).toBe(false);
    expect(validRule({ tags: [1, 1] })).toBe(false);
    expect(validRule({ tags: Array.from({ length: 17 }, (_, i) => i + 1) })).toBe(false);
    expect(validRule({ minDuration: 601, maxDuration: 600 })).toBe(false);
    expect(validRule({ addedWithin: 0 })).toBe(false);
    // 種類・人物・団体の id は正の整数だけ。
    expect(validRule({ tagKind: 0 })).toBe(false);
    expect(validRule({ person: -1 })).toBe(false);
    expect(validRule({ organization: 1.5 })).toBe(false);
    expect(validRule({ tagKind: 1, person: 2, organization: 3 })).toBe(true);
  });

  it("鍵は空の条件と鍵の順を無視して比べる", () => {
    expect(ruleKey({ tags: [1], watch: "watched", q: "" })).toBe(
      ruleKey({ watch: "watched", tags: [1] }),
    );
  });

  it("ランダムは保存できないので追加日の新しい順にする", () => {
    expect(folderSort("random")).toEqual({ sort: "addedDesc", randomReplaced: true });
    expect(folderSort("titleAsc")).toEqual({ sort: "titleAsc", randomReplaced: false });
  });
});

describe("親の木", () => {
  // 猫 ─ 未視聴 ─ 最近、犬（名前の自然順に並べ直す）。
  const folders: SmartFolderSummary[] = [
    { id: 4, name: "犬" },
    { id: 3, parentId: 2, name: "最近" },
    { id: 2, parentId: 1, name: "未視聴" },
    { id: 1, name: "猫" },
  ];

  it("木の順と深さで並べる", () => {
    expect(parentChoices(folders).map(({ folder, depth }) => [folder.id, depth])).toEqual(
      [
        [4, 0],
        [1, 0],
        [2, 1],
        [3, 2],
      ],
    );
  });

  it("直しているフォルダとその子孫は並べない（自分の中へは移せない）", () => {
    expect(parentChoices(folders, 1).map(({ folder }) => folder.id)).toEqual([4]);
    expect(parentChoices(folders, 2).map(({ folder }) => folder.id)).toEqual([4, 1]);
  });

  it("子孫をどの深さも数える", () => {
    expect([...descendantIds(folders, 1)].sort()).toEqual([2, 3]);
    expect(descendantIds(folders, 4).size).toBe(0);
  });

  it("壊れた木（親子の輪）でも数え終わる", () => {
    const loop: SmartFolderSummary[] = [
      { id: 1, parentId: 2, name: "a" },
      { id: 2, parentId: 1, name: "b" },
    ];
    expect([...descendantIds(loop, 1)].sort()).toEqual([1, 2]);
  });
});
