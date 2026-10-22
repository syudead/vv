import { describe, expect, it } from "vitest";

import { conditionChips, hasMissing, refText } from "./conditions";

describe("conditionChips", () => {
  it("条件の編集の行の順にチップを並べる", () => {
    const chips = conditionChips({
      organization: 9,
      person: 8,
      tags: [3, 4],
      tagKind: 2,
      favorite: true,
      q: "kyoto",
    });

    expect(chips.map((chip) => chip.key)).toEqual([
      "q",
      "favorite",
      "tagKind:2",
      "tag:3",
      "tag:4",
      "person:8",
      "organization:9",
    ]);
    expect(chips.flatMap((chip) => (chip.kind === "text" ? [chip.text] : []))).toEqual([
      "Search: kyoto",
      "Favorites",
    ]);
  });

  it("空の条件はチップを持たない", () => {
    expect(conditionChips({})).toEqual([]);
  });

  it("欠けた参照に名指されたチップだけに印を付ける", () => {
    const chips = conditionChips({ tags: [3, 4] }, [{ kind: "tag", id: 4 }]);

    expect(chips).toEqual([
      { kind: "ref", key: "tag:3", ref: "tag", id: 3, missing: false },
      { kind: "ref", key: "tag:4", ref: "tag", id: 4, missing: true },
    ]);
    expect(hasMissing(chips)).toBe(true);
    expect(hasMissing(conditionChips({ tags: [3] }, [{ kind: "person", id: 3 }]))).toBe(
      false,
    );
  });
});

describe("refText", () => {
  it("種類ごとの語を名前の前に置く", () => {
    expect(refText("tag", "猫")).toBe("Tag: 猫");
    expect(refText("tagKind", "Genre")).toBe("Kind: Genre");
    expect(refText("person", "A")).toBe("Person: A");
    expect(refText("organization", "B")).toBe("Organization: B");
  });
});
