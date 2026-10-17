import type { PlaylistItem } from "../api/playlists";
import { formatDuration } from "../lib/format";
import { orderedItemDurationMs, orderedItemTitle } from "../videoList/ItemRowCells";
import { itemStepPath } from "./playlistPlayback";

// プレイリストの画面が項目から決める規則。
// 画面の部品から外し、ロジックのテストで確かめる（docs/design-docs/web-testing.md）。

/** playlistPath はプレイリストの画面の場所である。 */
export function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

/**
 * itemTitle は項目の名前である。動画は題名、グループは名前。応答に中身が無い項目
 * （契約上は起きない）は空にする。
 */
export const itemTitle: (item: PlaylistItem) => string = orderedItemTitle;

/** itemDurationMs は項目の長さである。動画の長さか、グループの合計。分からなければ undefined。 */
export const itemDurationMs: (item: PlaylistItem) => number | undefined =
  orderedItemDurationMs;

/**
 * playlistDuration は見せる項目の長さの合計を「1:32:10」の形で返す。項目が無いか、長さの
 * 分からない項目が 1 つでもあれば空にし、見出しは件数だけを出す。
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
 * グループとスマートフォルダは `openVideoId` を開き（フォルダは `smartFolder` も付ける）、
 * `state.from` はプレイリストの画面にする。始める動画の
 * 無い項目（動画の無いフォルダ）は null。
 */
export function playTarget(playlistId: number, item: PlaylistItem): PlayTarget | null {
  const to = itemStepPath(playlistId, item);
  if (to === null) return null;
  return { to, state: { from: playlistPath(playlistId), autoplay: true } };
}

/**
 * firstPlayTarget は、始める動画のある最初の項目の行き先である。どの項目にも無ければ null。
 */
export function firstPlayTarget(
  playlistId: number,
  items: readonly PlaylistItem[],
): PlayTarget | null {
  for (const item of items) {
    const target = playTarget(playlistId, item);
    if (target !== null) return target;
  }
  return null;
}

/**
 * moveItem は `itemId` を `afterItemId` のすぐ後ろ（null なら先頭）へ移した並びを返す。
 * サーバーの move と同じ動きで、どちらかが並びに無ければ元の並びを返す。
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
 * どちらも無ければ null（見出しへ移す）。
 */
export function focusAfterRemove<T extends { id: number }>(
  items: readonly T[],
  removedId: number,
): number | null {
  const index = items.findIndex((item) => item.id === removedId);
  if (index < 0) return null;
  return items[index + 1]?.id ?? items[index - 1]?.id ?? null;
}
