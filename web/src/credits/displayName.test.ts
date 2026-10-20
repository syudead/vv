import { describe, expect, it } from "vitest";

import { creditInitials } from "./displayName";

describe("creditInitials", () => {
  it.each([
    ["中村 楓", "中楓"],
    ["aoi sato extra", "AS"],
    ["  Mio  ", "M"],
  ])("takes the first letter of up to two words of %j", (name, initials) => {
    expect(creditInitials({ name })).toBe(initials);
  });

  it("gives a provisional person none", () => {
    expect(creditInitials({ name: "", provisional: true })).toBe("");
  });
});
