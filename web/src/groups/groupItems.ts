import type { GroupItem } from "../api/groups";
import {
  favoriteTargets,
  type LibrarySelection,
  type SelectedGroup,
} from "../library/selection";

/** Deduplicate IDs across page boundaries because concurrent reordering can return an item twice. */
export function appendItems(
  current: readonly GroupItem[],
  page: readonly GroupItem[],
): GroupItem[] {
  const known = new Set(current.map((item) => item.id));
  return [...current, ...page.filter((item) => !known.has(item.id))];
}

/**
 * Retain selected videos on unloaded pages and reconcile child-group membership. Library
 * reconciliation cannot be reused because child-group members may also appear as direct videos;
 * preserve identity if unchanged.
 */
export function reconcileGroupSelection(
  selection: LibrarySelection,
  videoIds: readonly number[],
  items: readonly GroupItem[],
): LibrarySelection {
  const present = new Set(videoIds);
  const ids = new Set([...selection.ids].filter((id) => present.has(id)));
  const listed = new Map<number, readonly number[]>();
  for (const item of items) {
    if (item.kind === "group" && item.group !== undefined) {
      listed.set(item.group.id, item.group.videoIds);
    }
  }
  const groups = new Map<number, SelectedGroup>();
  let changed = ids.size !== selection.ids.size;
  for (const [key, group] of selection.groups) {
    const observed = listed.get(key);
    if (observed === undefined || !observed.every((id) => ids.has(id))) {
      changed = true;
      continue;
    }
    if (!sameIds(observed, group.videoIds)) changed = true;
    groups.set(key, { id: key, videoIds: observed });
  }
  return changed ? { ids, groups } : selection;
}

function sameIds(a: readonly number[], b: readonly number[]): boolean {
  return a.length === b.length && a.every((id, index) => b[index] === id);
}

/**
 * Return loaded selected items in display order, excluding videos covered by selected groups;
 * unloaded item IDs are unavailable.
 */
export function selectedItemIds(
  selection: LibrarySelection,
  items: readonly GroupItem[],
): number[] {
  const targets = favoriteTargets(selection);
  const videos = new Set(targets.videoIds);
  const groups = new Set(targets.groupIds);
  return items
    .filter((item) =>
      item.kind === "group"
        ? item.group !== undefined && groups.has(item.group.id)
        : item.video !== undefined && videos.has(item.video.id),
    )
    .map((item) => item.id);
}
