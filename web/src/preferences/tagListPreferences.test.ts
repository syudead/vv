import { describe, expect, it } from "vitest";

import { readTagListPreferences, writeTagListPreferences } from "./tagListPreferences";

function memoryStorage(initial?: string): Storage {
  const values = new Map<string, string>();
  if (initial !== undefined) values.set("vv.tags.v1", initial);
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => void values.delete(key),
    setItem: (key, value) => void values.set(key, value),
  };
}

describe("tagListPreferences", () => {
  it("保存が無ければ名前の順", () => {
    expect(readTagListPreferences(memoryStorage())).toEqual({ sort: "name" });
  });

  it("書いた並び順を読み戻す", () => {
    const storage = memoryStorage();
    writeTagListPreferences({ sort: "createdAsc" }, storage);
    expect(readTagListPreferences(storage)).toEqual({ sort: "createdAsc" });
  });

  it("壊れた保存・知らない値は名前の順", () => {
    for (const raw of [
      "{",
      "null",
      "[]",
      '"countDesc"',
      '{"sort":"random"}',
      '{"sort":3}',
    ]) {
      expect(readTagListPreferences(memoryStorage(raw))).toEqual({ sort: "name" });
    }
  });

  it("読み書きできない保存でも投げない", () => {
    const broken = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
    } as unknown as Storage;
    expect(readTagListPreferences(broken)).toEqual({ sort: "name" });
    expect(() => writeTagListPreferences({ sort: "countDesc" }, broken)).not.toThrow();
  });
});
