import { describe, expect, it, vi } from "vitest";

import { readPlaybackQuality, writePlaybackQuality } from "./playbackQuality";

const storageKey = "vv.playback-quality.v1";

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

describe("readPlaybackQuality", () => {
  it("保存した画質を戻す", () => {
    expect(readPlaybackQuality(fake(() => JSON.stringify("480p")))).toBe("480p");
    expect(readPlaybackQuality(fake(() => JSON.stringify("original")))).toBe("original");
  });

  it("保存値が無い・壊れている・知らない値・読めない場合は元の画質を返す", () => {
    expect(readPlaybackQuality(fake(() => null))).toBe("original");
    expect(readPlaybackQuality(fake(() => "{"))).toBe("original");
    expect(readPlaybackQuality(fake(() => JSON.stringify("240p")))).toBe("original");
    expect(readPlaybackQuality(fake(() => JSON.stringify({ quality: "480p" })))).toBe(
      "original",
    );
    expect(readPlaybackQuality(fake(throws))).toBe("original");
  });
});

describe("writePlaybackQuality", () => {
  it("画質を書き、読み戻せる", () => {
    const values = new Map<string, string>();
    const storage = fake(
      () => values.get(storageKey) ?? null,
      vi.fn((key: string, value: string) => {
        values.set(key, value);
      }) as unknown as () => void,
    );
    writePlaybackQuality("360p", storage);
    expect(values.get(storageKey)).toBe(JSON.stringify("360p"));
    expect(readPlaybackQuality(storage)).toBe("360p");
  });

  it("書き込みに失敗しても外へ投げない", () => {
    expect(() =>
      writePlaybackQuality(
        "480p",
        fake(() => null, throws),
      ),
    ).not.toThrow();
  });
});
