import { describe, expect, it } from "vitest";

import {
  addedLengthCount,
  addedOptions,
  clearedAddedLength,
  lengthOptions,
  parseAddedLength,
  validAddedLength,
  withLength,
} from "./addedLengthCriteria";

function parse(search: string) {
  return parseAddedLength(new URLSearchParams(search));
}

describe("parseAddedLength", () => {
  it("範囲の中の値を読む", () => {
    expect(parse("addedWithin=7&minDuration=60&maxDuration=600")).toEqual({
      filter: { addedWithin: 7, minDuration: 60, maxDuration: 600 },
      dropped: false,
    });
    expect(parse("")).toEqual({ filter: {}, dropped: false });
  });

  it("範囲の外や整数でない値はその値だけを落とし、落としたと知らせる", () => {
    expect(parse("addedWithin=0&maxDuration=600")).toEqual({
      filter: { addedWithin: undefined, minDuration: undefined, maxDuration: 600 },
      dropped: true,
    });
    expect(parse("addedWithin=36501").dropped).toBe(true);
    expect(parse("minDuration=-1").dropped).toBe(true);
    expect(parse("minDuration=1.5").dropped).toBe(true);
    expect(parse("maxDuration=360001").dropped).toBe(true);
  });

  it("下限が上限を超えるときは長さの両方を落とす", () => {
    expect(parse("addedWithin=30&minDuration=601&maxDuration=600")).toEqual({
      filter: { addedWithin: 30, minDuration: undefined, maxDuration: undefined },
      dropped: true,
    });
  });
});

describe("validAddedLength", () => {
  it("範囲と下限・上限の順を確かめる", () => {
    expect(validAddedLength({ addedWithin: 1, minDuration: 0, maxDuration: 0 })).toBe(
      true,
    );
    expect(validAddedLength({ addedWithin: 0 })).toBe(false);
    expect(validAddedLength({ minDuration: 10, maxDuration: 9 })).toBe(false);
    expect(validAddedLength({ minDuration: -1 })).toBe(false);
    expect(validAddedLength({ maxDuration: 360001 })).toBe(false);
    expect(validAddedLength({ maxDuration: 1.5 })).toBe(false);
  });
});

describe("選択肢", () => {
  it("Added は選択肢の日数を選んだ形にし、選択肢に無い日数は最後に足す", () => {
    expect(addedOptions(undefined).value).toBe("any");
    expect(addedOptions(7).value).toBe("7");
    const custom = addedOptions(14);
    expect(custom.options.map((option) => option.label)).toEqual([
      "Any time",
      "Last 24 hours",
      "Last 7 days",
      "Last 30 days",
      "Last 90 days",
      "Last year",
      "Last 14 days",
    ]);
    expect(custom.value).toBe("custom");
  });

  it("Length は表の範囲を選んだ形にし、選択肢に無い範囲は名前を付けて最後に足す", () => {
    expect(lengthOptions({}).value).toBe("any");
    const preset = lengthOptions({ minDuration: 300, maxDuration: 1199 });
    expect(preset.options.find((option) => option.value === preset.value)?.label).toBe(
      "5 to 20 minutes",
    );
    const custom = lengthOptions({ minDuration: 90 });
    expect(custom.options.at(-1)?.label).toBe("Over 1.5 minutes");
    expect(custom.value).toBe("custom");
    expect(
      lengthOptions({ minDuration: 60, maxDuration: 600 }).options.at(-1)?.label,
    ).toBe("1 to 10 minutes");
    expect(lengthOptions({ maxDuration: 150 }).options.at(-1)?.label).toBe(
      "Under 2.5 minutes",
    );
  });

  it("ボタンは追加日と長さを 1 つずつ数え、Clear filters は 3 つとも外す", () => {
    expect(
      addedLengthCount({ addedWithin: 7, minDuration: 300, maxDuration: 1199 }),
    ).toBe(2);
    expect(addedLengthCount({ maxDuration: 299 })).toBe(1);
    expect(clearedAddedLength).toEqual({
      addedWithin: [],
      minDuration: [],
      maxDuration: [],
    });
    expect(withLength({ minDuration: 3600 })).toEqual({
      minDuration: ["3600"],
      maxDuration: [],
    });
  });
});
