import { describe, expect, it } from "vitest";

import { person } from "../../../testing/personPage";
import { buildPersonPatch, digitsOnly, draftFromPerson } from "./personDraft";

describe("digitsOnly", () => {
  it("keeps an overlong pasted number a finite integer, so it is not sent as null", () => {
    const kept = digitsOnly("9".repeat(400));

    expect(Number.isSafeInteger(Number(kept))).toBe(true);
  });
});

describe("buildPersonPatch", () => {
  const loaded = draftFromPerson(
    person({ name: "Aoi", names: [{ name: "Aoi", language: "ja" }, { name: "あおい" }] }),
  );

  it("sends the whole list as names, the display name first with its language, and never name", () => {
    const draft = { ...loaded, name: "Aoi Sora", aliases: loaded.aliases.slice(0, 0) };

    const { patch, sent } = buildPersonPatch(draft, loaded);

    expect(patch).toEqual({ names: [{ name: "Aoi Sora", language: "ja" }] });
    expect(sent.nameKeys).toEqual([""]);
  });

  it("sends no names while the rows are as loaded", () => {
    expect(buildPersonPatch(loaded, loaded).patch).toEqual({});
  });
});
