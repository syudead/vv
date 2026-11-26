import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { compareTagsForSort, placeTag } from "./listRows";

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    kind: null,
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

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
