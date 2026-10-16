import { describe, expect, it } from "vitest";

import type { LibrarySmartFolder } from "../api/smartFolders";
import { chunks, latestAnswer, withAnswer } from "./cardValueRules";

describe("chunks", () => {
  it("60 件ずつに分ける", () => {
    const ids = Array.from({ length: 125 }, (_, index) => index + 1);
    expect(chunks(ids).map((chunk) => chunk.length)).toEqual([60, 60, 5]);
    expect(chunks([])).toEqual([]);
  });
});

describe("withAnswer", () => {
  it("答えた値を重ね、答えに入らなかったフォルダを gone にする", () => {
    const card = { id: 1, name: "猫", videoCount: 2 } as LibrarySmartFolder;
    const before = new Map([[9, card]]);

    const next = withAnswer(before, [1, 2], [card]);

    expect(next.get(1)).toBe(card);
    expect(next.get(2)).toBe("gone");
    expect(next.get(9)).toBe(card);
    expect(before.has(1)).toBe(false);
  });
});

describe("latestAnswer", () => {
  it("後から読み直している id の分を答えから外す", () => {
    const card = (id: number) => ({ id, name: String(id) }) as LibrarySmartFolder;
    const answered = latestAnswer([1, 2, 3], [card(1), card(2)], (id) => id !== 2);
    expect(answered.asked).toEqual([1, 3]);
    expect(answered.cards.map((each) => each.id)).toEqual([1]);
  });
});
