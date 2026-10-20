import { describe, expect, it } from "vitest";

import { personFactGroups, personFigures } from "./personFacts";

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

describe("personFigures", () => {
  const values = (profile: Parameters<typeof personFigures>[0]) =>
    personFigures(profile, today).map((item) => [item.label, item.value]);

  it("draws only the filled figures, the cup with the measurements", () => {
    expect(values({ heightCm: 158, bustCm: 84, hipsCm: 85, cup: "C" })).toEqual([
      ["Height", "158 cm"],
      ["Measurements (cm) · C cup", "84–––85"],
    ]);
  });

  it("shows a cup alone when no measurement is known", () => {
    expect(values({ cup: "C" })).toEqual([["Cup", "C"]]);
  });

  it("counts the age to the death date and names it so", () => {
    expect(values({ birthDate: "1950-05-01", deathDate: "2020-03-10" })).toEqual([
      ["Age at death", "69"],
    ]);
    expect(values({ birthDate: "1990", careerStartYear: 2014 })).toEqual([
      ["Age", "34–35"],
      ["Career", "Since 2014"],
    ]);
  });
});
