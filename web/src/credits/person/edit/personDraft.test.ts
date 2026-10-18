import { describe, expect, it } from "vitest";

import { digitsOnly } from "./personDraft";

describe("digitsOnly", () => {
  it("keeps an overlong pasted number a finite integer, so it is not sent as null", () => {
    const kept = digitsOnly("9".repeat(400));

    expect(Number.isSafeInteger(Number(kept))).toBe(true);
  });
});
