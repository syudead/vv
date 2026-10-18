/**
 * Match domain.FoldForMatch with NFKC, simple per-code-point lowercase, then hiragana-to-katakana
 * conversion. Both runtimes verify internal/domain/testdata/fold_for_match.json.
 */
export function foldForMatch(s: string): string {
  let out = "";
  for (const ch of s.normalize("NFKC")) {
    let cp = lowerCodePoint(ch.codePointAt(0)!);
    // Add 0x60 to U+3041-U+3096 and U+309D-U+309E for their katakana equivalents.
    if ((cp >= 0x3041 && cp <= 0x3096) || cp === 0x309d || cp === 0x309e) cp += 0x60;
    out += String.fromCodePoint(cp);
  }
  return out;
}

/**
 * U+0130 is the only unconditional SpecialCasing lowercase mapping that expands to multiple code
 * points.
 */
const simpleLower: ReadonlyMap<number, number> = new Map([[0x0130, 0x0069]]);

/**
 * Lowercase one code point at a time to match Go's unicode.ToLower without contextual sigma rules;
 * use simpleLower for expansions.
 */
export function lowerCodePoint(cp: number): number {
  const lowered = String.fromCodePoint(cp).toLowerCase();
  const first = lowered.codePointAt(0)!;
  if (lowered.length === String.fromCodePoint(first).length) return first;
  return simpleLower.get(cp) ?? cp;
}
