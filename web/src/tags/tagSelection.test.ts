import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import {
  countSelectable,
  keepLoaded,
  selectAllCheck,
  selectedKinds,
  withoutIds,
} from "./tagSelection";

function tag(id: number, tentative = false): Tag {
  return {
    id,
    name: `Tag ${String(id)}`,
    synonyms: [],
    videoCount: 0,
    tentative,
    createdAt: "2026-01-01T00:00:00Z",
  };
}

describe("keepLoaded", () => {
  it("読み込んだ行に無い id と改名中の行を外し、何も外れなければ同じ集合を返す", () => {
    const rows = [tag(1), tag(2), tag(3)];
    const current = new Set([1, 2, 9]);
    expect([...keepLoaded(current, rows, 2)]).toEqual([1]);
    const kept = new Set([1, 3]);
    expect(keepLoaded(kept, rows, 2)).toBe(kept);
  });
});

describe("countSelectable と selectAllCheck", () => {
  it("改名中の行は「すべて選ぶ」の対象に数えない", () => {
    const rows = [tag(1), tag(2), tag(3)];
    expect(countSelectable(rows, null)).toBe(3);
    expect(countSelectable(rows, 2)).toBe(2);
    expect(countSelectable(rows, 9)).toBe(3);
  });

  it("選んだ数が対象の数に届けば選択済み、途中なら中間", () => {
    expect(selectAllCheck(0, 3)).toBe(false);
    expect(selectAllCheck(2, 3)).toBe("indeterminate");
    expect(selectAllCheck(3, 3)).toBe(true);
  });
});

describe("selectedKinds", () => {
  it("選んだ行の中の仮と確定の有無を返す", () => {
    const rows = [tag(1, true), tag(2), tag(3, true)];
    expect(selectedKinds(rows, new Set())).toEqual({
      tentative: false,
      confirmed: false,
    });
    expect(selectedKinds(rows, new Set([1, 3]))).toEqual({
      tentative: true,
      confirmed: false,
    });
    expect(selectedKinds(rows, new Set([1, 2]))).toEqual({
      tentative: true,
      confirmed: true,
    });
  });
});

describe("withoutIds", () => {
  it("選択から ids を外す", () => {
    expect([...withoutIds(new Set([1, 2, 3]), new Set([2, 9]))]).toEqual([1, 3]);
  });
});
