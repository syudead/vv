import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import {
  isTagListSort,
  sortTags,
  tagSortDirection,
  tagSortKind,
  withTagSortDirection,
} from "./tagListOrder";

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const tags = [
  tag({ id: 1, name: "tag10", videoCount: 2, createdAt: "2026-01-02T00:00:00Z" }),
  tag({ id: 2, name: "tag2", videoCount: 2, createdAt: "2026-01-02T00:00:00Z" }),
  tag({ id: 3, name: "Beta", videoCount: 0, createdAt: "2026-01-03T00:00:00Z" }),
  tag({ id: 4, name: "alpha", videoCount: 9, createdAt: "2026-01-01T00:00:00Z" }),
];

const names = (list: Tag[]) => list.map((item) => item.name);

describe("sortTags", () => {
  it("名前は自然順で並べ、元の配列を変えない", () => {
    expect(names(sortTags(tags, "name"))).toEqual(["alpha", "Beta", "tag2", "tag10"]);
    expect(names(tags)).toEqual(["tag10", "tag2", "Beta", "alpha"]);
  });

  it("本数は向きどおりに並べ、同じ本数は名前の順にする", () => {
    expect(names(sortTags(tags, "countDesc"))).toEqual([
      "alpha",
      "tag2",
      "tag10",
      "Beta",
    ]);
    expect(names(sortTags(tags, "countAsc"))).toEqual(["Beta", "tag2", "tag10", "alpha"]);
  });

  it("作った日は時刻で並べ、同じ秒は名前の順にする", () => {
    expect(names(sortTags(tags, "createdDesc"))).toEqual([
      "Beta",
      "tag2",
      "tag10",
      "alpha",
    ]);
    expect(names(sortTags(tags, "createdAsc"))).toEqual([
      "alpha",
      "tag2",
      "tag10",
      "Beta",
    ]);
  });
});

describe("並び順の種類と向き", () => {
  it("種類と向きを読み、向きを変える", () => {
    expect(tagSortKind("countAsc")).toBe("count");
    expect(tagSortDirection("name")).toBeUndefined();
    expect(tagSortDirection("createdDesc")).toBe("desc");
    expect(withTagSortDirection("countDesc", "asc")).toBe("countAsc");
    expect(withTagSortDirection("createdAsc", "desc")).toBe("createdDesc");
    expect(withTagSortDirection("name", "asc")).toBe("name");
  });

  it("5 つの値だけを並び順として受ける", () => {
    for (const value of ["name", "countDesc", "countAsc", "createdDesc", "createdAsc"]) {
      expect(isTagListSort(value)).toBe(true);
    }
    for (const value of ["nameAsc", "toString", "", 1, null, undefined]) {
      expect(isTagListSort(value)).toBe(false);
    }
  });
});
