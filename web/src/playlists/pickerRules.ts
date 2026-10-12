import type { PlaylistItemsAdded, PlaylistSummary } from "../api/playlists";
import { t, type UiText } from "../i18n";

// 「Add to playlist」のピッカーの規則（specs/044-playlists/ui-design.md「Add to playlist」）。

/** maxNameLength はプレイリストの名前の上限（トリム後の符号位置の数、research.md R-11）。 */
const maxNameLength = 100;

/** nameTooLong はトリムした名前が上限を超えるか（サーバーと同じく符号位置で数える）。 */
export function nameTooLong(trimmed: string): boolean {
  return Array.from(trimmed).length > maxNameLength;
}

/** matchPlaylists は名前に打った文字（大文字小文字を区別しない）を含むものを API の順に返す。 */
export function matchPlaylists(
  playlists: readonly PlaylistSummary[],
  trimmed: string,
): PlaylistSummary[] {
  if (trimmed === "") return [...playlists];
  const needle = trimmed.toLowerCase();
  return playlists.filter((playlist) => playlist.name.toLowerCase().includes(needle));
}

/** resultText は追加の応答 `added` と `existing` からトーストの 1 文を作る（「Words」）。 */
export function resultText(result: PlaylistItemsAdded, name: string): UiText {
  const w = t.playlists.add;
  if (result.added === 0 && result.existing === 0) return w.nothing;
  if (result.existing === 0) return w.added(result.added, name);
  if (result.added === 0) return w.existing(result.existing, name);
  return w.partly(result.added, result.existing, name);
}
