import { describe, expect, it } from "vitest";

import { nextAfterRemoval, withKind } from "./useTagKindsTab";

const kind = (id: number, name: string) => ({
  id,
  name,
  tagCount: 0,
  createdAt: "2026-01-01T00:00:00Z",
});

describe("nextAfterRemoval（消えた行の次のフォーカス先）", () => {
  const order = [kind(1, "Character"), kind(2, "Genre"), kind(3, "Work")];

  it("次の行、最後の行なら前の行、1 行だけなら無し", () => {
    expect(nextAfterRemoval(order, 2)?.id).toBe(3);
    expect(nextAfterRemoval(order, 3)?.id).toBe(2);
    expect(nextAfterRemoval([kind(1, "Genre")], 1)).toBeUndefined();
  });

  it("並びに無い id なら無し（フォーカスは「New kind」へ）", () => {
    expect(nextAfterRemoval(order, 9)).toBeUndefined();
  });
});

describe("withKind", () => {
  it("同じ id の行を置き換え、名前の位置へ並べ直す", () => {
    const result = withKind([kind(1, "Character"), kind(2, "Genre")], kind(1, "Work"));
    expect(result.map((item) => item.name)).toEqual(["Genre", "Work"]);
  });
});
