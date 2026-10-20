import { describe, expect, it } from "vitest";

import {
  compareRows,
  faceChoices,
  initialImage,
  initialItems,
  type PersonCompareCurrent,
} from "./personCompareRows";

const current: PersonCompareCurrent = {
  name: "Aoi",
  aliases: [{ name: "Sora" }],
  externalIds: [{ source: "fanza", value: "a1" }],
  profile: { heightCm: 160 },
  imageRevision: 2,
};

const fetched = {
  name: "Aoi",
  // The person's own name counts as an alias already on the person.
  aliases: [{ name: "Aoi" }, { name: "Sora" }],
  externalIds: [
    { source: "fanza", value: "a1" },
    { source: "fanza", value: "b2" },
  ],
  profile: { heightCm: 158, cup: "C" },
  imageCount: 3,
};

describe("compareRows", () => {
  it("fills what is missing, keeps what the person has, and draws no row for absent values", () => {
    const rows = compareRows(current, fetched);

    expect(rows).toEqual([
      { item: "profile.heightCm", kind: "kept" },
      { item: "profile.cup", kind: "fill" },
      { item: "aliases", kind: "kept" },
      { item: "externalIds", kind: "fill" },
    ]);
    expect(initialItems(rows)).toEqual(["profile.cup", "externalIds"]);
  });

  it("offers a different stored value for replacement, unselected, and keeps an equal one", () => {
    const rows = compareRows(current, fetched, true);
    const same = compareRows(
      current,
      { name: "Aoi", aliases: [], externalIds: [], profile: { heightCm: 160 } },
      true,
    );

    expect(rows.slice(0, 2)).toEqual([
      { item: "profile.heightCm", kind: "replace" },
      { item: "profile.cup", kind: "fill" },
    ]);
    expect(initialItems(rows)).toEqual(["profile.cup", "externalIds"]);
    expect(same).toEqual([{ item: "profile.heightCm", kind: "kept" }]);
  });
});

describe("face choices", () => {
  it("offers every image to replace a face but none to fill one the person has", () => {
    expect([
      faceChoices(current, fetched, true),
      initialImage(current, fetched, true),
    ]).toEqual([3, null]);
    expect([faceChoices(current, fetched), initialImage(current, fetched)]).toEqual([
      0,
      null,
    ]);
  });

  it("starts a person without a face on the first image", () => {
    const faceless = { ...current, imageRevision: undefined };

    expect([faceChoices(faceless, fetched), initialImage(faceless, fetched)]).toEqual([
      3, 0,
    ]);
  });
});
