import { describe, expect, it, vi } from "vitest";

import {
  defaultSubtitlePreference,
  readSubtitlePreference,
  writeSubtitlePreference,
} from "./subtitlePreference";

const storageKey = "vv.subtitles.v1";

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

describe("readSubtitlePreference", () => {
  it("保存した選択を戻す", () => {
    expect(
      readSubtitlePreference(fake(() => JSON.stringify({ enabled: true, label: "ja" }))),
    ).toEqual({ enabled: true, label: "ja" });
    expect(
      readSubtitlePreference(fake(() => JSON.stringify({ enabled: false, label: "" }))),
    ).toEqual({ enabled: false, label: "" });
  });

  it("保存値が無い、壊れている、または読めない場合はオフを返す", () => {
    expect(defaultSubtitlePreference).toEqual({ enabled: false, label: "" });
    expect(readSubtitlePreference(fake(() => null))).toEqual(defaultSubtitlePreference);
    expect(readSubtitlePreference(fake(() => "{"))).toEqual(defaultSubtitlePreference);
    expect(readSubtitlePreference(fake(() => "[]"))).toEqual(defaultSubtitlePreference);
    expect(readSubtitlePreference(fake(() => "null"))).toEqual(defaultSubtitlePreference);
    expect(
      readSubtitlePreference(fake(() => JSON.stringify({ enabled: "yes", label: "ja" }))),
    ).toEqual(defaultSubtitlePreference);
    expect(
      readSubtitlePreference(fake(() => JSON.stringify({ enabled: true, label: 3 }))),
    ).toEqual(defaultSubtitlePreference);
    expect(readSubtitlePreference(fake(throws))).toEqual(defaultSubtitlePreference);
  });
});

describe("writeSubtitlePreference", () => {
  it("選択を書く", () => {
    const setItem = vi.fn();
    writeSubtitlePreference(
      { enabled: true, label: "en" },
      fake(() => null, setItem),
    );
    expect(setItem).toHaveBeenCalledWith(
      storageKey,
      JSON.stringify({ enabled: true, label: "en" }),
    );
  });

  it("書き込みに失敗しても外へ投げない", () => {
    expect(() =>
      writeSubtitlePreference(
        { enabled: true, label: "en" },
        fake(() => null, throws),
      ),
    ).not.toThrow();
  });
});
