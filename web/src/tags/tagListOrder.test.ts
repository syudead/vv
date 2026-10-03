import { describe, expect, it } from "vitest";

import {
  isTagListSort,
  tagSortDirection,
  tagSortKind,
  withTagSortDirection,
} from "./tagListOrder";

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
