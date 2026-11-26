import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { idsBetween, keepLoaded, selectedKinds, withoutIds } from "./tagSelection";

function tag(id: number, tentative = false): Tag {
  return {
    kind: null,
    id,
    name: `Tag ${String(id)}`,
    synonyms: [],
    videoCount: 0,
    tentative,
    createdAt: "2026-01-01T00:00:00Z",
  };
}

describe("keepLoaded", () => {
  it("読み込んだカードに無い id を外し、何も外れなければ同じ集合を返す", () => {
    const rows = [tag(1), tag(2), tag(3)];
    expect([...keepLoaded(new Set([1, 2, 9]), rows)]).toEqual([1, 2]);
    const kept = new Set([1, 3]);
    expect(keepLoaded(kept, rows)).toBe(kept);
  });
});

describe("idsBetween", () => {
  it("一覧の順で両端を含む範囲を返し、どちらかが読み込まれていなければ空", () => {
    const rows = [tag(1), tag(2), tag(3), tag(4)];
    expect(idsBetween(rows, 3, 1)).toEqual([1, 2, 3]);
    expect(idsBetween(rows, 2, 2)).toEqual([2]);
    expect(idsBetween(rows, 2, 9)).toEqual([]);
  });
});

describe("selectedKinds", () => {
  it("選んだ行の中の仮と確定の有無を返す", () => {
    const rows = [tag(1, true), tag(2), tag(3, true)];
    expect(selectedKinds(rows, new Set())).toEqual({
      tentative: false,
      confirmed: false,
      kind: undefined,
    });
    expect(selectedKinds(rows, new Set([1, 3]))).toEqual({
      tentative: true,
      confirmed: false,
      kind: "none",
    });
    expect(selectedKinds(rows, new Set([1, 2]))).toEqual({
      tentative: true,
      confirmed: true,
      kind: "none",
    });
  });

  it("選んだタグがみな同じ種類ならその id、混ざっていれば undefined を返す", () => {
    const genre = { id: 1, name: "Genre", cardFirst: false };
    const work = { id: 2, name: "Work", cardFirst: false };
    const rows = [
      { ...tag(1), kind: genre },
      { ...tag(2), kind: genre },
      { ...tag(3), kind: work },
      tag(4),
    ];
    expect(selectedKinds(rows, new Set([1, 2])).kind).toBe("1");
    expect(selectedKinds(rows, new Set([1, 3])).kind).toBeUndefined();
    expect(selectedKinds(rows, new Set([1, 4])).kind).toBeUndefined();
  });
});

describe("withoutIds", () => {
  it("選択から ids を外す", () => {
    expect([...withoutIds(new Set([1, 2, 3]), new Set([2, 9]))]).toEqual([1, 3]);
  });
});
