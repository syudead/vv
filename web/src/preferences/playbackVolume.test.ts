import { describe, expect, it, vi } from "vitest";

import {
  defaultPlaybackVolume,
  readPlaybackVolume,
  writePlaybackVolume,
} from "./playbackVolume";

const storageKey = "vv.playback-volume.v1";

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

describe("readPlaybackVolume", () => {
  it("保存した音量とミュート状態を戻す", () => {
    expect(
      readPlaybackVolume(fake(() => JSON.stringify({ volume: 0.35, muted: true }))),
    ).toEqual({ volume: 0.35, muted: true });
  });

  it("壊れた項目だけ既定値に戻す", () => {
    expect(
      readPlaybackVolume(fake(() => JSON.stringify({ volume: 2, muted: true }))),
    ).toEqual({ volume: 1, muted: true });
    expect(
      readPlaybackVolume(fake(() => JSON.stringify({ volume: 0.2, muted: "yes" }))),
    ).toEqual({ volume: 0.2, muted: false });
  });

  it("保存値が無い、壊れている、または読めない場合は既定値を返す", () => {
    expect(readPlaybackVolume(fake(() => null))).toEqual(defaultPlaybackVolume);
    expect(readPlaybackVolume(fake(() => "{"))).toEqual(defaultPlaybackVolume);
    expect(readPlaybackVolume(fake(throws))).toEqual(defaultPlaybackVolume);
  });
});

describe("writePlaybackVolume", () => {
  it("音量とミュート状態を書く", () => {
    const setItem = vi.fn();
    writePlaybackVolume(
      { volume: 0.6, muted: false },
      fake(() => null, setItem),
    );
    expect(setItem).toHaveBeenCalledWith(
      storageKey,
      JSON.stringify({ volume: 0.6, muted: false }),
    );
  });

  it("書き込みに失敗しても外へ投げない", () => {
    expect(() =>
      writePlaybackVolume(
        { volume: 0.6, muted: false },
        fake(() => null, throws),
      ),
    ).not.toThrow();
  });
});
