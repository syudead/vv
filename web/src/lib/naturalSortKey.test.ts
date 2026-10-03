// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { compareNaturalSortKeys, naturalSortKey } from "./naturalSortKey";

/**
 * Go の NaturalSortKey と同じ入力の組を読む（specs/036-tag-admin-scale/research.md
 * R-12「移植の検査」）。Go 側は internal/domain/foldcheck/natural_sort_key_test.go が
 * 同じファイルを読み、組が鍵の strings.Compare の順に並んでいることも確かめる。
 */
interface NaturalSortKeyFile {
  cases: { note: string; input: string; want: string }[];
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
      "natural_sort_key.json",
    ),
    "utf8",
  ),
) as NaturalSortKeyFile;

describe("naturalSortKey", () => {
  it("共有の組で Go の NaturalSortKey と同じ鍵を返す", () => {
    expect(file.cases.length).toBeGreaterThan(0);
    for (const c of file.cases) {
      expect(naturalSortKey(c.input), c.note).toBe(c.want);
    }
  });

  it("数字の連続を桁数の接頭辞つきに置き換える", () => {
    expect(naturalSortKey("tag 2")).toBe("tag 00012");
    expect(naturalSortKey("tag 10")).toBe("tag 000210");
    expect(naturalSortKey("0")).toBe("0000");
    expect(naturalSortKey("00")).toBe("0000");
  });
});

describe("compareNaturalSortKeys", () => {
  it("共有の組の鍵を Go の strings.Compare と同じ順に並べる", () => {
    const rows = file.cases.map((c) => ({ note: c.note, key: naturalSortKey(c.input) }));
    rows.forEach((a, i) => {
      for (const b of rows.slice(i)) {
        const same = a.key === b.key;
        const pair = `${a.note} / ${b.note}`;
        expect(compareNaturalSortKeys(a.key, b.key), pair).toBe(same ? 0 : -1);
        expect(compareNaturalSortKeys(b.key, a.key), pair).toBe(same ? 0 : 1);
      }
    });
  });

  it("U+FFFF より大きい符号位置を U+E000〜U+FFFF の後ろに置く", () => {
    const pairs: [string, string][] = [
      ["", "𠮷"],
      ["�", "😀"],
      ["a￿", "a\u{10000}"],
    ];
    for (const [low, high] of pairs) {
      // UTF-16 のコード単位の順（`<`）では、サロゲートペアの側が前に来る。
      expect(high < low).toBe(true);
      expect(compareNaturalSortKeys(low, high)).toBe(-1);
      expect(compareNaturalSortKeys(high, low)).toBe(1);
    }
  });

  it("下位サロゲートだけが違う鍵も符号位置の順に比べる", () => {
    expect(compareNaturalSortKeys("\u{1F600}", "\u{1F601}")).toBe(-1);
    expect(compareNaturalSortKeys("\u{20BB7}", "\u{20BB7}")).toBe(0);
  });

  it("一方が他方の先頭と同じなら短いほうが前", () => {
    expect(compareNaturalSortKeys("tag", "tag 00012")).toBe(-1);
    expect(compareNaturalSortKeys("", "a")).toBe(-1);
  });
});
