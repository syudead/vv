import { describe, expect, it } from "vitest";

import {
  compareRows,
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

describe("compareRows", () => {
  it("keeps what the person has, changes what is missing, and draws no row for absent values", () => {
    const rows = compareRows(current, {
      name: "Aoi",
      // The person's own name counts as an alias already on the person.
      aliases: [{ name: "Aoi" }, { name: "Sora" }],
      externalIds: [
        { source: "fanza", value: "a1" },
        { source: "fanza", value: "b2" },
      ],
      profile: { heightCm: 158, cup: "C" },
      hasImage: true,
    });

    expect(rows).toEqual([
      { item: "image", changes: false },
      { item: "profile.heightCm", changes: false },
      { item: "profile.cup", changes: true },
      { item: "aliases", changes: false },
      { item: "externalIds", changes: true },
    ]);
  });

  it("offers stored values that differ for replacement, unselected, and keeps equal ones", () => {
    const rows = compareRows(
      current,
      {
        name: "Aoi",
        aliases: [],
        externalIds: [],
        profile: { heightCm: 158, cup: "C" },
        hasImage: true,
      },
      true,
    );
    const same = compareRows(
      current,
      { name: "Aoi", aliases: [], externalIds: [], profile: { heightCm: 160 } },
      true,
    );

    expect(rows).toEqual([
      { item: "image", changes: true, replaces: true },
      { item: "profile.heightCm", changes: true, replaces: true },
      { item: "profile.cup", changes: true },
    ]);
    expect(initialItems(rows)).toEqual(["profile.cup"]);
    expect(same).toEqual([{ item: "profile.heightCm", changes: false }]);
  });
});
