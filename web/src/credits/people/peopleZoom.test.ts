import { describe, expect, it } from "vitest";

import { readPeopleZoom } from "./peopleZoom";

function storage(value: string | null): Storage {
  return { getItem: () => value } as unknown as Storage;
}

describe("readPeopleZoom", () => {
  it.each([
    [null, 1],
    ["", 1],
    ["7", 1],
    ["0", 0],
    ["3", 3],
  ])("reads %j as %j", (stored, zoom) => {
    expect(readPeopleZoom(storage(stored))).toBe(zoom);
  });
});
