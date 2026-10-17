import type { HomeShelf } from "../api/home";

// ホームのフォルダの棚の並びの規則（docs/design-docs/home.md「Home」、research.md R-7）。
// PUT /api/home/shelves はフォルダの棚の id を上から順に全部送るので、足す・外す・動かすは
// どれも今の並びから次の並びを作る。画面の部品から外し、ロジックのテストで確かめる
// （docs/design-docs/web-testing.md）。

/** folderShelfIds はホームの棚の一覧から、フォルダの棚の id を上から順に取り出す。 */
export function folderShelfIds(shelves: readonly HomeShelf[]): number[] {
  return shelves.flatMap((shelf) =>
    shelf.kind === "folder" && shelf.folderId !== undefined ? [shelf.folderId] : [],
  );
}

/**
 * withShelfMoved は `id` の棚を 1 つ上（-1）か下（1）へ動かした並びである。端より先へは
 * 動かさず、無い id ならそのままの並びを返す。
 */
export function withShelfMoved(
  ids: readonly number[],
  id: number,
  direction: -1 | 1,
): number[] {
  const next = [...ids];
  const from = next.indexOf(id);
  const to = from + direction;
  if (from < 0 || to < 0 || to >= next.length) return next;
  [next[from], next[to]] = [next[to]!, next[from]!];
  return next;
}

/**
 * withShelfRemoved は `id` の棚を外した並びと、外した棚がいた位置（「Undo」で戻す場所）である。
 * 無い id なら位置は -1。
 */
export function withShelfRemoved(
  ids: readonly number[],
  id: number,
): { ids: number[]; index: number } {
  return { ids: ids.filter((other) => other !== id), index: ids.indexOf(id) };
}

/**
 * withShelfAt は `id` の棚を `index` の位置に置いた並びである。`index` が並びより先（や負）なら
 * 最後に置く。もう並びにある id は動かさない。「Add shelf」は最後のフォルダの棚の後に足し、
 * 「Undo」は外す前の位置に戻す（ui-design.md「Words」）。
 */
export function withShelfAt(ids: readonly number[], id: number, index: number): number[] {
  if (ids.includes(id)) return [...ids];
  const at = index < 0 || index > ids.length ? ids.length : index;
  return [...ids.slice(0, at), id, ...ids.slice(at)];
}

/** ShelfMoves はフォルダの棚の「More」に出す動かし方である。 */
export interface ShelfMoves {
  up: boolean;
  down: boolean;
}

/**
 * shelfMoves は `id` の棚に「Move up」「Move down」を出すかである。最初のフォルダの棚に
 * 「Move up」は無く、最後のものに「Move down」は無い（ui-design.md「Words」）。
 */
export function shelfMoves(ids: readonly number[], id: number): ShelfMoves {
  const index = ids.indexOf(id);
  return { up: index > 0, down: index >= 0 && index < ids.length - 1 };
}
