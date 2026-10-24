import { describe, expect, it } from "vitest";

import { readTheaterMode, writeTheaterMode } from "./theaterMode";

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

describe("theaterMode", () => {
  it("書いた値を読み戻す", () => {
    writeTheaterMode(true, localStorage);
    expect(readTheaterMode(localStorage)).toBe(true);
    writeTheaterMode(false, localStorage);
    expect(readTheaterMode(localStorage)).toBe(false);
  });

  it("保存値が無い、壊れている、または読めない場合はオフを返す", () => {
    expect(readTheaterMode(fake(() => null))).toBe(false);
    expect(readTheaterMode(fake(() => "yes"))).toBe(false);
    expect(readTheaterMode(fake(throws))).toBe(false);
  });

  it("書けないときは例外を出さず、書けなかったことを返す", () => {
    expect(
      writeTheaterMode(
        true,
        fake(() => null, throws),
      ),
    ).toBe(false);
  });
});
