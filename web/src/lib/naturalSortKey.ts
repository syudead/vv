import { foldForMatch } from "./foldForMatch";

/**
 * naturalSortKey はサーバーの `domain.NaturalSortKey`（internal/domain/search.go）と
 * 同じ名前の自然順の鍵を作る。`foldForMatch` を掛けたうえで、ASCII の数字の連続を
 * 「先頭の 0 を除いた桁数を 10 進 4 桁で表した接頭辞 + 先頭の 0 を除いた数字」に
 * 置き換える（`2` → `00012`、`10` → `000210`、`0` や `00` は `0000`。
 * specs/013-library-search/data-model.md §4）。
 *
 * 鍵は `compareNaturalSortKeys` で比べる。画面が読み込んだ行の中で行を差し込み・
 * 動かす位置を、サーバーの名前の順（`sort_key`）と同じにするための移植である
 * （specs/036-tag-admin-scale/research.md R-12）。Go と同じ鍵になることは
 * internal/domain/testdata/natural_sort_key.json の組で、Go と Vitest の両方から確かめる。
 */
export function naturalSortKey(name: string): string {
  return foldForMatch(name).replace(/[0-9]+/g, (digits) => {
    const trimmed = digits.replace(/^0+/, "");
    return String(trimmed.length).padStart(4, "0") + trimmed;
  });
}

/**
 * compareNaturalSortKeys は 2 つの鍵を符号位置の順で比べ、-1・0・1 を返す。
 * Go は鍵を UTF-8 のバイト順で比べ、これは符号位置の順と一致する。UTF-16 の
 * コード単位の順（`<` での文字列の比較）は、U+FFFF より大きい符号位置（サロゲート
 * ペア）を U+E000〜U+FFFF の前に置いて食い違うので、使わない。
 */
export function compareNaturalSortKeys(a: string, b: string): number {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) {
    const x = a.charCodeAt(i);
    const y = b.charCodeAt(i);
    if (x !== y) return Math.sign(codePointOrder(x) - codePointOrder(y));
  }
  return Math.sign(a.length - b.length);
}

/**
 * codePointOrder は UTF-16 のコード単位を、最初に食い違った位置での比較が符号位置の
 * 順になる値へ移す。サロゲート（U+D800〜U+DFFF）を U+E000〜U+FFFF の後ろへ、
 * U+E000〜U+FFFF をその分だけ前へずらす。食い違いが下位サロゲートどうしで起きても、
 * 両方が同じだけずれるので前後は変わらない。
 */
function codePointOrder(unit: number): number {
  if (unit >= 0xe000) return unit - 0x800;
  if (unit >= 0xd800) return unit + 0x2000;
  return unit;
}
