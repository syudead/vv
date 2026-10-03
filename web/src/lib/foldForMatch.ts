/**
 * foldForMatch はサーバーの `domain.FoldForMatch`（internal/domain/search.go）と
 * 同じ照合形を作る。NFKC 正規化、1 符号位置ずつの小文字化、ひらがなから
 * カタカナへの置き換えの順に掛ける。全角半角・大文字小文字・かなの違いと、
 * NFC・NFD の違いが同じ照合形に畳まれる（specs/036-tag-admin-scale/research.md R-3）。
 *
 * Go と同じ結果になることは internal/domain/testdata/fold_for_match.json の
 * 組で、Go と Vitest の両方から確かめる。
 */
export function foldForMatch(s: string): string {
  let out = "";
  for (const ch of s.normalize("NFKC")) {
    let cp = lowerCodePoint(ch.codePointAt(0)!);
    // U+3041–U+3096 と ゝゞ（U+309D・U+309E）は、0x60 足すと対応する
    // カタカナ（ァ–ヶ、ヽヾ）になる。
    if ((cp >= 0x3041 && cp <= 0x3096) || cp === 0x309d || cp === 0x309e) cp += 0x60;
    out += String.fromCodePoint(cp);
  }
  return out;
}

/**
 * simpleLower は、1 文字の `toLowerCase` が複数の符号位置になる文字の単純な
 * 小文字である。SpecialCasing の無条件の小文字化で複数になるのは U+0130 だけ。
 */
const simpleLower: ReadonlyMap<number, number> = new Map([[0x0130, 0x0069]]);

/**
 * lowerCodePoint は Go の `unicode.ToLower` と同じ単純な小文字化である。
 * 1 文字だけの文字列に `toLowerCase` を掛けるので語末の文脈は無く、Σ は σ に
 * なる。結果が 1 符号位置でなければ `simpleLower` を引き、無ければ変えない。
 */
export function lowerCodePoint(cp: number): number {
  const lowered = String.fromCodePoint(cp).toLowerCase();
  const first = lowered.codePointAt(0)!;
  if (lowered.length === String.fromCodePoint(first).length) return first;
  return simpleLower.get(cp) ?? cp;
}
