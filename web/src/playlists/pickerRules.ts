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

/** matchNames は名前に打った文字（大文字小文字を区別しない）を含むものを読んだ順に返す。 */
export function matchNames<T extends { name: string }>(
  rows: readonly T[],
  trimmed: string,
): T[] {
  if (trimmed === "") return [...rows];
  const needle = trimmed.toLowerCase();
  return rows.filter((row) => row.name.toLowerCase().includes(needle));
}

/** resultText は追加の応答 `added` と `existing` からトーストの 1 文を作る（「Words」）。 */
export function resultText(result: PlaylistItemsAdded, name: string): UiText {
  const w = t.playlists.add;
  if (result.added === 0 && result.existing === 0) return w.nothing;
  if (result.existing === 0) return w.added(result.added, name);
  if (result.added === 0) return w.existing(result.existing, name);
  return w.partly(result.added, result.existing, name);
}
