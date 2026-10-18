import { describe, expect, it } from "vitest";

import type { PlaylistSummary } from "../api/playlists";
import { matchNames, nameTooLong, resultText } from "./pickerRules";

function playlist(id: number, name: string): PlaylistSummary {
  return { id, name, itemCount: 0, updatedAt: "2026-10-01T00:00:00Z" };
}

describe("ピッカーの規則", () => {
  it("トーストは added と existing で文を分ける（「Words」）", () => {
    expect(resultText({ added: 3, existing: 0 }, "Tonight")).toBe(
      'Added 3 items to "Tonight"',
    );
    expect(resultText({ added: 1, existing: 0 }, "Tonight")).toBe(
      'Added 1 item to "Tonight"',
    );
    expect(resultText({ added: 0, existing: 1 }, "Tonight")).toBe('Already in "Tonight"');
    expect(resultText({ added: 0, existing: 2 }, "Tonight")).toBe(
      '2 items already in "Tonight"',
    );
    expect(resultText({ added: 2, existing: 1 }, "Tonight")).toBe(
      'Added 2 items to "Tonight"; 1 already there',
    );
    expect(resultText({ added: 0, existing: 0 }, "Tonight")).toBe("Nothing was added");
  });

  it("打った文字を大文字小文字を区別せずに含むものを、API の順のまま返す", () => {
    const all = [playlist(3, "Tonight"), playlist(2, "Anime"), playlist(1, "night owl")];
    expect(matchNames(all, "").map((p) => p.id)).toEqual([3, 2, 1]);
    expect(matchNames(all, "NIGHT").map((p) => p.id)).toEqual([3, 1]);
    expect(matchNames(all, "zzz")).toEqual([]);
  });

  it("大文字小文字はサーバーと同じ Unicode の単純な折り畳みで比べる", () => {
    const all = [playlist(1, "Σ mix"), playlist(2, "\u212Aelvin"), playlist(3, "straße")];
    // Simple folding joins final sigma with sigma; lowercase alone does not.
    expect(matchNames(all, "ς").map((p) => p.id)).toEqual([1]);
    expect(matchNames(all, "σ").map((p) => p.id)).toEqual([1]);
    expect(matchNames(all, "kel").map((p) => p.id)).toEqual([2]);
    // Simple folding excludes expansions such as sharp-s to SS and the Turkish dotless-I pair.
    expect(matchNames(all, "strasse")).toEqual([]);
    expect(
      matchNames([playlist(4, "\u0131"), playlist(5, "I")], "i").map((p) => p.id),
    ).toEqual([5]);
  });

  it("名前の上限は符号位置で 100 まで", () => {
    expect(nameTooLong("a".repeat(100))).toBe(false);
    expect(nameTooLong("a".repeat(101))).toBe(true);
    // Count supplementary emoji as one code point, not two UTF-16 units.
    expect(nameTooLong("😀".repeat(100))).toBe(false);
  });
});
