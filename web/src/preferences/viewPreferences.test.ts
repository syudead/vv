import { describe, expect, it, vi } from "vitest";

import { videoSorts } from "../api/client";
import { defaults, readViewPreferences, writeViewPreferences } from "./viewPreferences";

const storageKey = "vv.view.v2";

function fake(getItem: () => string | null, setItem: () => void = () => {}): Storage {
  return {
    getItem,
    setItem,
    removeItem: () => {},
    clear: () => {},
    key: () => null,
    length: 0,
  } as Storage;
}

function throws(): never {
  throw new Error("localStorage は使えません");
}

describe("readViewPreferences", () => {
  it("getItem が投げても既定値を返す", () => {
    expect(readViewPreferences(fake(throws))).toEqual(defaults);
  });

  it("鍵が無い・JSON でない・オブジェクトでない場合は既定値", () => {
    expect(readViewPreferences(fake(() => null))).toEqual(defaults);
    expect(readViewPreferences(fake(() => "{ zoom:"))).toEqual(defaults);
    expect(readViewPreferences(fake(() => "[1]"))).toEqual(defaults);
  });

  it("壊れた項目だけ既定値に落とす", () => {
    expect(
      readViewPreferences(
        fake(() => JSON.stringify({ zoom: 9, view: "list", sort: "titleAsc" })),
      ),
    ).toEqual({ zoom: 1, view: "list", sort: "titleAsc" });
    expect(
      readViewPreferences(
        fake(() => JSON.stringify({ zoom: 3, view: "wall", sort: "nope" })),
      ),
    ).toEqual({ zoom: 3, view: "grid", sort: "addedDesc" });
  });
});

describe("保存できる並び順", () => {
  it("API の 13 の並び順をすべて戻す", () => {
    for (const sort of videoSorts) {
      expect(
        readViewPreferences(fake(() => JSON.stringify({ zoom: 1, view: "grid", sort })))
          .sort,
      ).toBe(sort);
    }
  });
});

describe("writeViewPreferences", () => {
  it("鍵と JSON で書く", () => {
    const setItem = vi.fn();
    writeViewPreferences(
      { zoom: 2, view: "list", sort: "titleAsc" },
      fake(() => null, setItem),
    );
    expect(setItem).toHaveBeenCalledWith(
      storageKey,
      JSON.stringify({ zoom: 2, view: "list", sort: "titleAsc" }),
    );
  });

  it("setItem が投げても外へ出さない", () => {
    expect(() =>
      writeViewPreferences(
        defaults,
        fake(() => null, throws),
      ),
    ).not.toThrow();
  });
});
