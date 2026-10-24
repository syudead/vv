import { describe, expect, it } from "vitest";

import {
  compareRows,
  initialSelection,
  type PersonCompareCurrent,
  selectionBody,
  skippedPersonReason,
} from "./personCompareRows";

const current: PersonCompareCurrent = {
  name: "Aoi",
  aliases: [{ name: "Sora" }],
  externalIds: [{ source: "fanza", value: "a1" }],
  profile: { heightCm: 160, links: ["https://a.test"] },
  imageRevision: 2,
};

const fetched = {
  name: "Aoi Sora",
  aliases: [{ name: "Aoi" }, { name: "Sora" }, { name: "Aoichan" }],
  externalIds: [
    { source: "fanza", value: "a1" },
    { source: "fanza", value: "b2" },
  ],
  profile: { heightCm: 158, cup: "C", links: ["https://a.test", "https://b.test"] },
  imageCount: 3,
};

describe("compareRows", () => {
  it("fills what is missing, keeps what the person has, and leaves lists and links out", () => {
    expect(compareRows(current, fetched)).toEqual([
      { item: "profile.heightCm", kind: "kept" },
      { item: "profile.cup", kind: "fill" },
    ]);
  });

  it("offers a different stored value for replacement and keeps an equal one", () => {
    const same = compareRows(
      current,
      { name: "Aoi", aliases: [], externalIds: [], profile: { heightCm: 160 } },
      true,
    );

    expect(compareRows(current, fetched, true)[0]).toEqual({
      item: "profile.heightCm",
      kind: "replace",
    });
    expect(same).toEqual([{ item: "profile.heightCm", kind: "kept" }]);
  });
});

describe("initialSelection", () => {
  it("selects filling fields and every value the lists would add, but no replacement or stored face", () => {
    const selection = initialSelection(current, fetched, true);

    // The fetched name is an alias choice; names and IDs the person has are not.
    expect(selection).toEqual({
      items: ["profile.cup"],
      image: null,
      aliases: ["Aoi Sora", "Aoichan"],
      externalIds: [{ source: "fanza", value: "b2" }],
      links: ["https://b.test"],
    });
    expect(selectionBody({ ...selection, image: 2 }).items).toEqual([
      "profile.cup",
      "image",
    ]);
  });

  it("starts a person without a face on the first image, and a created person without the fetched name", () => {
    const created = { aliases: [], externalIds: [], profile: {} };

    const selection = initialSelection(created, fetched);

    expect(selection.image).toBe(0);
    expect(selection.aliases).toEqual(["Aoi", "Sora", "Aoichan"]);
  });

  it("offers no image to fill a face the person has", () => {
    expect(initialSelection(current, fetched).image).toBeNull();
  });
});

describe("skippedPersonReason", () => {
  it("names the list whose limit each too_many value went over", () => {
    const reason = (item: "aliases" | "externalIds" | "profile.links") =>
      skippedPersonReason({ item, reason: "too_many" });

    expect(reason("aliases")).toBe("over the limit for aliases");
    expect(reason("externalIds")).toBe("over the limit for external IDs");
    expect(reason("profile.links")).toBe("over the limit for links");
  });
});
