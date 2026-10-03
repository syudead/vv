import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import {
  addTag,
  appendUniqueTags,
  compareTagsForSort,
  confirmTags,
  placeTag,
  removeTags,
  replaceTag,
  type TagPageRows,
  type TagRowsQuery,
} from "./tagPageRows";

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const query = (overrides: Partial<TagRowsQuery> = {}): TagRowsQuery => ({
  query: "",
  tentativeOnly: false,
  unusedOnly: false,
  sort: "name",
  ...overrides,
});

function page(rows: Tag[], overrides: Partial<TagPageRows> = {}): TagPageRows {
  return {
    rows,
    total: rows.length,
    totalAll: rows.length,
    nextCursor: undefined,
    boundary: undefined,
    query: query(),
    ...overrides,
  };
}

const names = (result: TagPageRows) => result.rows.map((item) => item.name);

describe("compareTagsForSort", () => {
  it("名前の順は名前の自然順の鍵で比べ、同じ鍵は id で決める", () => {
    const items = [
      tag({ id: 3, name: "B10" }),
      tag({ id: 2, name: "ｂ２" }),
      tag({ id: 1, name: "b2" }),
      tag({ id: 4, name: "B1" }),
    ];
    expect(
      items.sort((a, b) => compareTagsForSort(a, b, "name")).map((t) => t.id),
    ).toEqual([4, 1, 2, 3]);
  });

  it("本数・作った日の順は先にその値で比べ、作った日は秒で比べる", () => {
    const a = tag({
      id: 1,
      name: "B",
      videoCount: 1,
      createdAt: "2026-01-01T00:00:00.900Z",
    });
    const b = tag({
      id: 2,
      name: "A",
      videoCount: 3,
      createdAt: "2026-01-01T00:00:00.100Z",
    });
    expect(compareTagsForSort(a, b, "countDesc")).toBe(1);
    expect(compareTagsForSort(a, b, "countAsc")).toBe(-1);
    // 同じ秒なので名前の順。
    expect(compareTagsForSort(a, b, "createdDesc")).toBe(1);
  });
});

describe("placeTag と続きの境", () => {
  const loaded = [tag({ id: 1, name: "A" }), tag({ id: 2, name: "C" })];

  it("続きが無ければ末尾にも置く", () => {
    expect(
      placeTag(loaded, tag({ id: 3, name: "D" }), "name", undefined).map((t) => t.name),
    ).toEqual(["A", "C", "D"]);
  });

  it("境より後ろに並ぶ行は置かず、境の前なら置く", () => {
    const boundary = loaded[1];
    expect(placeTag(loaded, tag({ id: 3, name: "D" }), "name", boundary)).toHaveLength(2);
    expect(
      placeTag(loaded, tag({ id: 3, name: "B" }), "name", boundary).map((t) => t.name),
    ).toEqual(["A", "B", "C"]);
  });

  it("境の行そのものは、鍵が変わらなければ置き直しても残る", () => {
    const boundary = loaded[1]!;
    const confirmed = { ...boundary, tentative: false };
    expect(placeTag(loaded, confirmed, "name", boundary)).toHaveLength(2);
  });
});

describe("操作のあとの反映", () => {
  it("作成は条件に合えば位置に置いて total・totalAll を増やし、合わなければ totalAll だけ", () => {
    const base = page([tag({ id: 1, name: "Alpha" })], {
      query: query({ query: "al" }),
    });
    const matched = addTag(base, tag({ id: 2, name: "Also" }));
    expect(names(matched)).toEqual(["Alpha", "Also"]);
    expect([matched.total, matched.totalAll]).toEqual([2, 2]);
    const other = addTag(base, tag({ id: 3, name: "Zulu" }));
    expect(names(other)).toEqual(["Alpha"]);
    expect([other.total, other.totalAll]).toEqual([1, 2]);
  });

  it("改名で検索に合わなくなった行は取り除き total を減らす", () => {
    const base = page([tag({ id: 1, name: "Alpha" }), tag({ id: 2, name: "Alps" })], {
      total: 2,
      totalAll: 10,
      query: query({ query: "alp" }),
    });
    const result = replaceTag(base, base.rows[0], tag({ id: 1, name: "Zeta" }));
    expect(names(result)).toEqual(["Alps"]);
    expect([result.total, result.totalAll]).toEqual([1, 10]);
  });

  it("「Tentative only」でまとめて確定した行は取り除き total を減らす", () => {
    const base = page(
      [
        tag({ id: 1, name: "A", tentative: true }),
        tag({ id: 2, name: "B", tentative: true }),
      ],
      { total: 5, totalAll: 9, query: query({ tentativeOnly: true }) },
    );
    const result = confirmTags(base, new Set([1]));
    expect(names(result)).toEqual(["B"]);
    expect(result.total).toBe(4);
  });

  it("削除は読み込んだ行から取り除き、total と totalAll を減らす", () => {
    const base = page([tag({ id: 1, name: "A" }), tag({ id: 2, name: "B" })], {
      total: 2,
      totalAll: 2,
    });
    const result = removeTags(base, [base.rows[0]!]);
    expect(names(result)).toEqual(["B"]);
    expect([result.total, result.totalAll]).toEqual([1, 1]);
  });

  describe("countDesc で読み込んでいない統合先へ統合する", () => {
    // サーバーには 6 個あり、本数の多い順の先頭の 3 個を読み込んである。
    const all = [
      tag({ id: 1, name: "A", videoCount: 10 }),
      tag({ id: 2, name: "B", videoCount: 8 }),
      tag({ id: 3, name: "C", videoCount: 6 }),
      tag({ id: 4, name: "D", videoCount: 4 }),
      tag({ id: 5, name: "E", videoCount: 2 }),
      tag({ id: 6, name: "F", videoCount: 1 }),
    ];
    const loaded = (): TagPageRows =>
      page(all.slice(0, 3), {
        total: 6,
        totalAll: 6,
        nextCursor: "cursor",
        boundary: all[2],
        query: query({ sort: "countDesc" }),
      });

    function merge(source: Tag, target: Tag): TagPageRows {
      const merged = { ...target, videoCount: target.videoCount + source.videoCount };
      return replaceTag(removeTags(loaded(), [source]), target, merged);
    }

    it("位置が読み込んだ範囲の中なら統合先を差し込み、続きを末尾まで読んでも二重に出ない", () => {
      // E（2 本）を B（8 本）へではなく、D（4 本）へ B を統合すると D は 12 本で先頭へ。
      const result = merge(all[1]!, all[3]!);
      expect(names(result)).toEqual(["D", "A", "C"]);
      expect([result.total, result.totalAll]).toEqual([5, 5]);
      // 続きはカーソル（C の鍵）より後ろを返す。サーバーでは D は前へ動いたので返さない
      // が、返したとしても id で捨てる。
      const rest = appendUniqueTags(result.rows, [
        { ...all[3]!, videoCount: 12 },
        all[4]!,
        all[5]!,
      ]);
      expect(rest.map((item) => item.name)).toEqual(["D", "A", "C", "E", "F"]);
    });

    it("位置が範囲の外なら差し込まず、件数だけを数え直す", () => {
      // F（1 本）を E（2 本）へ統合すると E は 3 本で、C（6 本）より後ろ。
      const result = merge(all[5]!, all[4]!);
      expect(names(result)).toEqual(["A", "B", "C"]);
      expect([result.total, result.totalAll]).toEqual([5, 5]);
    });

    it("統合の前に条件に合わず、統合で合うようになった統合先は total を増やす", () => {
      const base = page([tag({ id: 1, name: "Action" })], {
        total: 1,
        totalAll: 3,
        query: query({ query: "act" }),
      });
      const target = tag({ id: 2, name: "Film" });
      const merged = { ...target, synonyms: ["Action"] };
      const result = replaceTag(removeTags(base, [base.rows[0]!]), target, merged);
      expect(names(result)).toEqual(["Film"]);
      expect([result.total, result.totalAll]).toEqual([1, 2]);
    });
  });
});
