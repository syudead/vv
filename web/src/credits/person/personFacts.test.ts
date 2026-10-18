import { describe, expect, it } from "vitest";

import { personFactGroups } from "./personFacts";

// A local date: the age follows the viewer's calendar day.
const today = new Date(2025, 5, 15);

function fact(birthDate: string, deathDate?: string) {
  const groups = personFactGroups({ birthDate, deathDate }, today);
  return groups[0]?.facts.map((item) => [item.term, item.value]);
}

describe("personFactGroups", () => {
  it("shows a year-only birth date with the range of ages it allows", () => {
    expect(fact("1990")).toEqual([["Born", "1990 (age 34–35)"]]);
  });

  it("shows the age at death on the death date, not on today", () => {
    expect(fact("1950-05-01", "2020-03-10")).toEqual([
      ["Born", "1950-05-01"],
      ["Died", "2020-03-10 (aged 69)"],
    ]);
  });

  it("leaves out a group with no value", () => {
    const groups = personFactGroups({ heightCm: 160 }, today);
    expect(groups.map((group) => group.id)).toEqual(["measurements"]);
    expect(groups[0]?.facts.map((item) => item.value)).toEqual(["160 cm"]);
  });
});
