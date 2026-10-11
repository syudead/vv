import type { PlaylistItem } from "../api/playlists";
import { formatDuration } from "../lib/format";

// プレイリストの画面が項目から決める規則（specs/044-playlists/ui-design.md「Playlist screen」）。
// 画面の部品から外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** playlistPath はプレイリストの画面の場所である。 */
export function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

/**
 * itemTitle は項目の名前である。動画は題名、グループはフォルダ名。応答に中身が無い項目
 * （契約上は起きない）は空にする。
 */
export function itemTitle(item: PlaylistItem): string {
  if (item.kind === "group") return item.group?.name ?? "";
  return item.video?.title ?? "";
}

/** itemDurationMs は項目の長さである。動画の長さか、グループの合計。分からなければ undefined。 */
export function itemDurationMs(item: PlaylistItem): number | undefined {
  if (item.kind === "group") return item.group?.durationMs;
  return item.video?.durationMs;
}

/**
 * playlistDuration は見せる項目の長さの合計を「1:32:10」の形で返す。項目が無いか、長さの
 * 分からない項目が 1 つでもあれば空にし、見出しは件数だけを出す（ui-design.md「Words」）。
 */
export function playlistDuration(items: readonly PlaylistItem[]): string {
  if (items.length === 0) return "";
  let total = 0;
  for (const item of items) {
    const ms = itemDurationMs(item);
    if (ms === undefined) return "";
    total += ms;
  }
  return formatDuration(total);
}

/** PlayTarget は項目から再生を始める行き先（Link の to と state）である。 */
export interface PlayTarget {
  to: string;
  state: { from: string; autoplay: true };
}

/**
 * playTarget は項目 `item` から再生を始める動画ページの行き先である。動画はその動画、
 * グループは `openVideoId` を開き、`state.from` はプレイリストの画面にする
 * （contracts/screen-api.md「Client use」）。中身の無い項目は null。
 */
export function playTarget(playlistId: number, item: PlaylistItem): PlayTarget | null {
  const videoId = item.kind === "group" ? item.group?.openVideoId : item.video?.id;
  if (videoId === undefined) return null;
  const params = new URLSearchParams({
    playlist: String(playlistId),
    item: String(item.id),
  });
  return {
    to: `/videos/${String(videoId)}?${params.toString()}`,
    state: { from: playlistPath(playlistId), autoplay: true },
  };
}

/**
 * moveItem は `itemId` を `afterItemId` のすぐ後ろ（null なら先頭）へ移した並びを返す。
 * サーバーの move と同じ動き（research.md R-3）で、どちらかが並びに無ければ元の並びを返す。
 */
export function moveItem<T extends { id: number }>(
  items: readonly T[],
  itemId: number,
  afterItemId: number | null,
): readonly T[] {
  const moving = items.find((item) => item.id === itemId);
  if (moving === undefined || afterItemId === itemId) return items;
  const rest = items.filter((item) => item.id !== itemId);
  if (afterItemId === null) return [moving, ...rest];
  const index = rest.findIndex((item) => item.id === afterItemId);
  if (index < 0) return items;
  return [...rest.slice(0, index + 1), moving, ...rest.slice(index + 1)];
}

/** Drop は行を置いたときの、新しい並びと送る `afterItemId` である。 */
export interface Drop<T> {
  items: readonly T[];
  afterItemId: number | null;
}

/**
 * dropItem は `itemId` の行を、いま `overId` の行がある位置へ置いた結果である。同じ位置なら
 * null で、何も送らない。`afterItemId` は新しい位置の上にある項目、先頭なら null。
 */
export function dropItem<T extends { id: number }>(
  items: readonly T[],
  itemId: number,
  overId: number,
): Drop<T> | null {
  const from = items.findIndex((item) => item.id === itemId);
  const to = items.findIndex((item) => item.id === overId);
  if (from < 0 || to < 0 || from === to) return null;
  const next = [...items];
  const [moving] = next.splice(from, 1);
  if (moving === undefined) return null;
  next.splice(to, 0, moving);
  return { items: next, afterItemId: to === 0 ? null : (next[to - 1]?.id ?? null) };
}

/**
 * focusAfterRemove は外した行の後にフォーカスを移す行である。次の行、無ければ前の行、
 * どちらも無ければ null（見出しへ移す。ui-design.md「Removing an item」）。
 */
export function focusAfterRemove<T extends { id: number }>(
  items: readonly T[],
  removedId: number,
): number | null {
  const index = items.findIndex((item) => item.id === removedId);
  if (index < 0) return null;
  return items[index + 1]?.id ?? items[index - 1]?.id ?? null;
}
