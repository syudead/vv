import type { PlaylistItem } from "../api/playlists";
import { formatDuration } from "../lib/format";
import {
  isVirtualItem,
  orderedItemDurationMs,
  orderedItemTitle,
} from "../videoList/ItemRowCells";
import { itemStepPath } from "./playlistPlayback";

export function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

/** Return empty for a contract-violating item with no content. */
export const itemTitle: (item: PlaylistItem) => string = orderedItemTitle;

export const itemDurationMs: (item: PlaylistItem) => number | undefined =
  orderedItemDurationMs;

/** Omit duration when the playlist is empty or any item has unknown duration. */
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

export interface PlayTarget {
  to: string;
  state: { from: string; autoplay: true };
}

/** Preserve playlist return context and smart-folder context; empty folders have no target. */
export function playTarget(playlistId: number, item: PlaylistItem): PlayTarget | null {
  const to = itemStepPath(playlistId, item);
  if (to === null) return null;
  return { to, state: { from: playlistPath(playlistId), autoplay: true } };
}

/** Pass over virtual videos, which have nothing to play, as the server's continuous play does. */
export function firstPlayTarget(
  playlistId: number,
  items: readonly PlaylistItem[],
): PlayTarget | null {
  for (const item of items) {
    if (isVirtualItem(item)) continue;
    const target = playTarget(playlistId, item);
    if (target !== null) return target;
  }
  return null;
}

/** True for a nonempty playlist of only virtual videos; other unplayable mixes are not explained. */
export function allVirtual(items: readonly PlaylistItem[]): boolean {
  return items.length > 0 && items.every(isVirtualItem);
}

/** Match server move semantics; return the original order if either item is missing. */
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

export interface Drop<T> {
  items: readonly T[];
  afterItemId: number | null;
}

/** No-op for unchanged positions; afterItemId is the new predecessor, null at the start. */
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

/** Focus the next row, then previous row, then the heading via null. */
export function focusAfterRemove<T extends { id: number }>(
  items: readonly T[],
  removedId: number,
): number | null {
  const index = items.findIndex((item) => item.id === removedId);
  if (index < 0) return null;
  return items[index + 1]?.id ?? items[index - 1]?.id ?? null;
}
