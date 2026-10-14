import type { PlaylistItemsAdded } from "../api/playlists";
import { t, type UiText } from "../i18n";

// 「Add to playlist」と「Add to group」のピッカーの規則（specs/044-playlists/ui-design.md
// 「Add to playlist」、specs/048-groups/ui-design.md「Add to group」）。

/** maxNameLength はプレイリストの名前の上限（トリム後の符号位置の数、research.md R-11）。 */
const maxNameLength = 100;

/** nameTooLong はトリムした名前が上限を超えるか（サーバーと同じく符号位置で数える）。 */
export function nameTooLong(trimmed: string): boolean {
  return Array.from(trimmed).length > maxNameLength;
}

/** dotlessI と dottedI は Unicode の単純な折り畳みで `I`・`i` と組にならない（トルコ語の規則だけ）。 */
const dotlessI = "\u0131";
const dottedI = "\u0130";

/**
 * foldLinks は、大文字小文字の変換を片方へ辿るだけでは届かない組である（`ͅ` → `Ι` は
 * あっても `Ι` → `ͅ` は無い、など）。Unicode の CaseFolding.txt の単純な折り畳みから拾った。
 */
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

/** foldStep は 1 符号位置から大文字小文字の変換で 1 歩で届く 1 符号位置を返す。 */
function foldStep(char: string): string[] {
  const steps = [char.toLowerCase(), char.toUpperCase(), foldLinks.get(char) ?? char];
  // 2 符号位置以上になる変換（`ß` → `SS` など）は単純な折り畳みに入らない。
  return steps.filter(
    (step) => Array.from(step).length === 1 && step !== dotlessI && step !== dottedI,
  );
}

/**
 * foldChar は 1 符号位置を、大文字小文字で組になる符号位置（Go の unicode.SimpleFold が
 * 巡る組）のうち最も小さいものにする。
 */
function foldChar(char: string): string {
  const known = foldedChars.get(char);
  if (known !== undefined) return known;
  const least = leastInOrbit(char);
  foldedChars.set(char, least);
  return least;
}

/** foldedChars は foldChar の結果の控えである（打つたびに全部の名前を畳むので）。 */
const foldedChars = new Map<string, string>();

/** leastInOrbit は foldChar の中身で、組を辿って最も小さい符号位置を返す。 */
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
 * foldName は名前を大文字小文字を区別しない比較の鍵にする。サーバーの名前の絞り込み
 * （Go の domain.FoldKey、Unicode の単純な折り畳み）と同じ組を同じ鍵にするので、`Σ`・`σ`・`ς`
 * も `K` と Kelvin 記号も一致する。`toLowerCase` だけでは `σ` と `ς` が別になる。
 */
export function foldName(name: string): string {
  return Array.from(name, foldChar).join("");
}

/**
 * matchNames は名前に打った文字（大文字小文字を区別しない、foldName）を含むものを読んだ順に
 * 返す。
 */
export function matchNames<T extends { name: string }>(
  rows: readonly T[],
  trimmed: string,
): T[] {
  if (trimmed === "") return [...rows];
  const needle = foldName(trimmed);
  return rows.filter((row) => foldName(row.name).includes(needle));
}

/** resultText は追加の応答 `added` と `existing` からトーストの 1 文を作る（「Words」）。 */
export function resultText(result: PlaylistItemsAdded, name: string): UiText {
  const w = t.playlists.add;
  if (result.added === 0 && result.existing === 0) return w.nothing;
  if (result.existing === 0) return w.added(result.added, name);
  if (result.added === 0) return w.existing(result.existing, name);
  return w.partly(result.added, result.existing, name);
}
