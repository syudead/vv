import { describe, expect, it } from "vitest";

import type { ListCriteria } from "../videoList/listCriteria";
import {
  addTagId,
  clearConditions,
  hasConditions,
  MAX_TAG_COUNT,
  parseTagParam,
  removeTagId,
  serializeTagIds,
} from "./tagCriteria";

function baseCriteria(extra: Partial<ListCriteria> = {}): ListCriteria {
  return { query: "", watch: "all", playable: false, sort: "addedDesc", ...extra };
}

describe("parseTagParam", () => {
  it("数でない値・重複・17個目以降を捨てる", () => {
    const raw = [
      ...Array.from({ length: 18 }, (_, index) => String(index + 1)),
      "abc",
      "1",
      "-1",
      "3.5",
    ];
    const ids = parseTagParam(raw);
    expect(ids).toHaveLength(MAX_TAG_COUNT);
    expect(ids).toEqual(Array.from({ length: 16 }, (_, index) => index + 1));
  });

  it("誤りを出さず、空なら空配列", () => {
    expect(parseTagParam([])).toEqual([]);
    expect(parseTagParam(["", "  ", "abc"])).toEqual([]);
  });
});

describe("serializeTagIds", () => {
  it("id の昇順にそろえ、重複を1つにまとめる", () => {
    expect(serializeTagIds([8, 3, 3, 1])).toEqual(["1", "3", "8"]);
  });

  it("16個を超える分は切り捨てる", () => {
    const ids = Array.from({ length: 20 }, (_, index) => 20 - index);
    expect(serializeTagIds(ids)).toHaveLength(MAX_TAG_COUNT);
  });
});

describe("addTagId / removeTagId", () => {
  it("同じタグを足しても変わらない", () => {
    expect(addTagId([1, 2], 2)).toEqual([1, 2]);
  });

  it("16個あるときは17個目を足さない", () => {
    const full = Array.from({ length: MAX_TAG_COUNT }, (_, index) => index + 1);
    expect(addTagId(full, 999)).toEqual(full);
  });

  it("外すとその id だけが消える", () => {
    expect(removeTagId([1, 2, 3], 2)).toEqual([1, 3]);
  });
});

describe("hasConditions / clearConditions", () => {
  it("タグだけでも条件ありと判定する", () => {
    expect(hasConditions(baseCriteria(), [1])).toBe(true);
    expect(hasConditions(baseCriteria(), [])).toBe(false);
  });

  it("clearConditions は検索語などとタグの両方を外し、並べ替えは残す", () => {
    const result = clearConditions(
      baseCriteria({
        query: "abc",
        watch: "unwatched",
        playable: true,
        sort: "titleAsc",
      }),
    );
    expect(result.criteria).toEqual(baseCriteria({ sort: "titleAsc" }));
    expect(result.tag).toEqual([]);
  });
});
