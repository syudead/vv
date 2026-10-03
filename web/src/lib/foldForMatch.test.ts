// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { foldForMatch, lowerCodePoint } from "./foldForMatch";

/**
 * Go の FoldForMatch と同じ入力の組を読む（specs/036-tag-admin-scale/research.md R-3）。
 * Go 側は internal/domain/fold_for_match_test.go が同じファイルを読む。
 */
interface FoldForMatchFile {
  cases: { note: string; input: string; want: string }[];
  lower: { from: number; to: number }[];
}

const file = JSON.parse(
  readFileSync(
    join(
      __dirname,
      "..",
      "..",
      "..",
      "internal",
      "domain",
      "testdata",
      "fold_for_match.json",
    ),
    "utf8",
  ),
) as FoldForMatchFile;

const unassigned = /^\p{Cn}$/u;

describe("foldForMatch", () => {
  it("共有の組で Go の FoldForMatch と同じ照合形を返す", () => {
    expect(file.cases.length).toBeGreaterThan(0);
    for (const c of file.cases) {
      expect(foldForMatch(c.input), c.note).toBe(c.want);
    }
  });

  it("lowerCodePoint は表のすべての組で Go の unicode.ToLower と同じ符号位置を返す", () => {
    expect(file.lower.length).toBeGreaterThan(1000);
    // この JavaScript の実行系の Unicode の版がまだ持たない符号位置（Go の版で
    // 足された文字）は、R-3「残る差」の既知の例外として比べない。
    const known = file.lower.filter(
      ({ from }) => !unassigned.test(String.fromCodePoint(from)),
    );
    expect(known.length).toBeGreaterThan(1000);
    const mismatches = known.filter(({ from, to }) => lowerCodePoint(from) !== to);
    expect(mismatches).toEqual([]);
  });

  it("小文字化を持たない符号位置は変えない", () => {
    expect(lowerCodePoint(0x61)).toBe(0x61);
    expect(lowerCodePoint(0x30a2)).toBe(0x30a2);
  });
});
