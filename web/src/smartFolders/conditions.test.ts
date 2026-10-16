import { describe, expect, it } from "vitest";

import { conditionChips, hasMissing, refText } from "./conditions";

describe("conditionChips", () => {
  it("条件の編集の行の順にチップを並べる", () => {
    const chips = conditionChips({
      minDuration: 300,
      maxDuration: 1199,
      addedWithin: 7,
      organization: 9,
      person: 8,
      tags: [3, 4],
      tagKind: 2,
      playable: true,
      favorite: true,
      watch: "unwatched",
      q: "kyoto",
    });

    expect(chips.map((chip) => chip.key)).toEqual([
      "q",
      "watch",
      "favorite",
      "playable",
      "tagKind:2",
      "tag:3",
      "tag:4",
      "person:8",
      "organization:9",
      "addedWithin",
      "length",
    ]);
    expect(chips.flatMap((chip) => (chip.kind === "text" ? [chip.text] : []))).toEqual([
      "Search: kyoto",
      "Unwatched",
      "Favorites",
      "Playable",
      "Added in the last 7 days",
      "5 to 20 minutes",
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

  it("追加日と長さはプリセットの語、ほかは日数と分で言う", () => {
    const text = (rule: Parameters<typeof conditionChips>[0]) =>
      conditionChips(rule).map((chip) => (chip.kind === "text" ? chip.text : ""));

    expect(text({ addedWithin: 1 })).toEqual(["Added in the last 24 hours"]);
    expect(text({ addedWithin: 365 })).toEqual(["Added in the last year"]);
    expect(text({ addedWithin: 14 })).toEqual(["Added in the last 14 days"]);
    expect(text({ maxDuration: 299 })).toEqual(["Under 5 minutes"]);
    expect(text({ minDuration: 1200, maxDuration: 3599 })).toEqual(["20 to 60 minutes"]);
    expect(text({ minDuration: 3600 })).toEqual(["Over an hour"]);
    expect(text({ minDuration: 90 })).toEqual(["Over 1.5 minutes"]);
    expect(text({ minDuration: 1 })).toEqual(["Over 0.1 minutes"]);
    expect(text({ maxDuration: 599 })).toEqual(["Under 10 minutes"]);
    expect(text({ minDuration: 90, maxDuration: 599 })).toEqual(["1.5 to 10 minutes"]);
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
