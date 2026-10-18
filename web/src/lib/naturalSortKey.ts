import { foldForMatch } from "./foldForMatch";

/**
 * Match the server's natural sort key: fold names, then prefix digit runs with their four-digit
 * significant length. Compare using compareNaturalSortKeys; shared fixtures verify parity with Go.
 */
export function naturalSortKey(name: string): string {
  return foldForMatch(name).replace(/[0-9]+/g, (digits) => {
    const trimmed = digits.replace(/^0+/, "");
    return String(trimmed.length).padStart(4, "0") + trimmed;
  });
}

/**
 * Compare by code point to match Go's UTF-8 byte order; JavaScript's UTF-16 string comparison
 * misorders supplementary characters.
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

/** Shift surrogate units after U+E000-U+FFFF so the first differing unit preserves code-point order. */
function codePointOrder(unit: number): number {
  if (unit >= 0xe000) return unit - 0x800;
  if (unit >= 0xd800) return unit + 0x2000;
  return unit;
}
