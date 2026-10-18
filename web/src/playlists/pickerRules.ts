import type { PlaylistItemsAdded } from "../api/playlists";
import { t, type UiText } from "../i18n";

const maxNameLength = 100;

export function nameTooLong(trimmed: string): boolean {
  return Array.from(trimmed).length > maxNameLength;
}

/** Default Unicode simple folding excludes Turkish dotted and dotless I pairs. */
const dotlessI = "\u0131";
const dottedI = "\u0130";

/** CaseFolding.txt supplies orbit links not reachable by following case conversions in one direction. */
const foldPairs: readonly (readonly [number, number])[] = [
  [0x345, 0x3b9],
  [0xb5, 0x3bc],
  [0x390, 0x1fd3],
  [0x3b0, 0x1fe3],
  [0xfb05, 0xfb06],
];
const foldLinks = new Map<string, string>(
  foldPairs.flatMap(([a, b]): [string, string][] => {
    const [x, y] = [String.fromCodePoint(a), String.fromCodePoint(b)];
    return [
      [x, y],
      [y, x],
    ];
  }),
);

function foldStep(char: string): string[] {
  const steps = [char.toLowerCase(), char.toUpperCase(), foldLinks.get(char) ?? char];
  // Simple folding excludes mappings expanding to multiple code points.
  return steps.filter(
    (step) => Array.from(step).length === 1 && step !== dotlessI && step !== dottedI,
  );
}

/** Choose the smallest code point in the same orbit as Go's unicode.SimpleFold. */
function foldChar(char: string): string {
  const known = foldedChars.get(char);
  if (known !== undefined) return known;
  const least = leastInOrbit(char);
  foldedChars.set(char, least);
  return least;
}

/** Cache folds because each keystroke compares every name. */
const foldedChars = new Map<string, string>();

function leastInOrbit(char: string): string {
  if (char === dotlessI || char === dottedI) return char;
  const orbit = new Set([char]);
  const queue = [char];
  for (let next = queue.pop(); next !== undefined; next = queue.pop()) {
    for (const step of foldStep(next)) {
      if (orbit.has(step)) continue;
      orbit.add(step);
      queue.push(step);
    }
  }
  let least = char;
  for (const member of orbit) {
    if ((member.codePointAt(0) ?? 0) < (least.codePointAt(0) ?? 0)) least = member;
  }
  return least;
}

/**
 * Match Go's domain.FoldKey simple-fold orbits; lowercase alone does not unify normal and final
 * sigma.
 */
export function foldName(name: string): string {
  return Array.from(name, foldChar).join("");
}

export function matchNames<T extends { name: string }>(
  rows: readonly T[],
  trimmed: string,
): T[] {
  if (trimmed === "") return [...rows];
  const needle = foldName(trimmed);
  return rows.filter((row) => foldName(row.name).includes(needle));
}

export function resultText(result: PlaylistItemsAdded, name: string): UiText {
  const w = t.playlists.add;
  if (result.added === 0 && result.existing === 0) return w.nothing;
  if (result.existing === 0) return w.added(result.added, name);
  if (result.added === 0) return w.existing(result.existing, name);
  return w.partly(result.added, result.existing, name);
}
