import { describe, expect, it } from "vitest";

import { pickParams } from "./filterPicks";
import { MAX_TAG_COUNT } from "./tagCriteria";

describe("pickParams", () => {
  it("adds a tag to the sorted tag list, and leaves a tag already in it unchanged", () => {
    expect(pickParams({ kind: "tag", id: 3 }, [5, 1])).toEqual({
      kind: "params",
      params: { tag: ["1", "3", "5"] },
    });
    expect(pickParams({ kind: "tag", id: 5 }, [5, 1])).toEqual({ kind: "unchanged" });
  });

  it("refuses a tag beyond the limit", () => {
    const full = Array.from({ length: MAX_TAG_COUNT }, (_, index) => index + 1);
    expect(pickParams({ kind: "tag", id: 99 }, full)).toEqual({ kind: "tagLimit" });
  });

  it("writes one value for a kind, a person or an organization, replacing the held one", () => {
    const full = Array.from({ length: MAX_TAG_COUNT }, (_, index) => index + 1);
    expect(pickParams({ kind: "tagKind", id: 4 }, full)).toEqual({
      kind: "params",
      params: { tagKind: ["4"] },
    });
    expect(pickParams({ kind: "person", id: 7 }, [])).toEqual({
      kind: "params",
      params: { person: ["7"] },
    });
    expect(pickParams({ kind: "organization", id: 8 }, [])).toEqual({
      kind: "params",
      params: { organization: ["8"] },
    });
  });
});
