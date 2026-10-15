import { describe, expect, it } from "vitest";

import {
  creditSignature,
  hasCreditFilter,
  parseCreditFilter,
  parseCreditParam,
} from "./creditCriteria";

describe("creditCriteria", () => {
  it("正の整数だけを読み、いくつもあれば最初の正しい値を使う", () => {
    expect(parseCreditParam([])).toBeUndefined();
    expect(parseCreditParam(["abc", "0", "-1", "1.5"])).toBeUndefined();
    expect(parseCreditParam(["x", "7", "8"])).toBe(7);
    expect(parseCreditParam(["99999999999999999999"])).toBeUndefined();
  });

  it("URL の person と organization を読み、どちらかがあれば絞り込み中とみなす", () => {
    const filter = parseCreditFilter(new URLSearchParams("person=3&organization=4"));
    expect(filter).toEqual({ person: 3, organization: 4 });
    expect(hasCreditFilter(filter)).toBe(true);
    expect(hasCreditFilter(parseCreditFilter(new URLSearchParams("tag=1")))).toBe(false);
  });

  it("URL の tagKind を読み、それだけでも絞り込み中とみなす", () => {
    const filter = parseCreditFilter(new URLSearchParams("tagKind=5"));
    expect(filter).toEqual({ tagKind: 5 });
    expect(hasCreditFilter(filter)).toBe(true);
    expect(creditSignature(filter)).not.toBe(creditSignature({}));
  });
});
