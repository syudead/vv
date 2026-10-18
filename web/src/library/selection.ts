import type { LibraryGroup, LibraryItem } from "../api/client";

/** Group identities used by favorite and playlist requests. */
export interface SelectedGroup {
  id: number;
  videoIds: readonly number[];
}

/** Track member video IDs for tags/visibility and group identities for favorites/playlists. Removing a member drops its group identity; remaining members become standalone selections. */
export interface LibrarySelection {
  ids: ReadonlySet<number>;
  groups: ReadonlyMap<number, SelectedGroup>;
}

export const emptySelection: LibrarySelection = { ids: new Set(), groups: new Map() };

function removeIds(
  selection: LibrarySelection,
  removed: readonly number[],
): LibrarySelection {
  const ids = new Set(selection.ids);
  const gone = new Set<number>();
  for (const id of removed) {
    if (ids.delete(id)) gone.add(id);
  }
  if (gone.size === 0) return selection;
  const groups = new Map(selection.groups);
  for (const [key, group] of groups) {
    if (group.videoIds.some((id) => gone.has(id))) groups.delete(key);
  }
  return { ids, groups };
}

/** Restrict selection to loaded items so vanished videos cannot be mutated in batches. */
export function keepMembers(
  selection: LibrarySelection,
  members: ReadonlySet<number>,
): LibrarySelection {
  return removeIds(
    selection,
    [...selection.ids].filter((id) => !members.has(id)),
  );
}

export function setVideo(
  selection: LibrarySelection,
  id: number,
  selected: boolean,
): LibrarySelection {
  if (!selected) return removeIds(selection, [id]);
  if (selection.ids.has(id)) return selection;
  const ids = new Set(selection.ids);
  ids.add(id);
  return { ids, groups: selection.groups };
}

export function toggleVideo(selection: LibrarySelection, id: number): LibrarySelection {
  return setVideo(selection, id, !selection.ids.has(id));
}

// Keep members still selected through another group.
export function setGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
  selected: boolean,
): LibrarySelection {
  if (!selected) return deselectGroup(selection, group);
  const ids = new Set(selection.ids);
  for (const id of group.videoIds) ids.add(id);
  const groups = new Map(selection.groups);
  groups.set(group.id, copyGroup(group));
  return { ids, groups };
}

function deselectGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
): LibrarySelection {
  const kept = new Set<number>();
  for (const [key, other] of selection.groups) {
    if (key === group.id) continue;
    for (const id of other.videoIds) kept.add(id);
  }
  const removed = removeIds(
    selection,
    group.videoIds.filter((id) => !kept.has(id)),
  );
  if (!removed.groups.has(group.id)) return removed;
  const groups = new Map(removed.groups);
  groups.delete(group.id);
  return { ids: removed.ids, groups };
}

function copyGroup(group: SelectedGroup): SelectedGroup {
  return { id: group.id, videoIds: group.videoIds };
}

export function selectedGroupOf(group: LibraryGroup): SelectedGroup {
  return copyGroup(group);
}

// Selection follows group identity, not whether every member remains selected through other groups.
export function isGroupSelected(selection: LibrarySelection, groupId: number): boolean {
  return selection.groups.has(groupId);
}

export function toggleGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
): LibrarySelection {
  return setGroup(selection, group, !isGroupSelected(selection, group.id));
}

/** Reconcile loaded groups without pruning video IDs or unloaded selections. Drop identities with standalone members or unselected new members; preserve the reference when unchanged. */
export function reconcileGroups(
  selection: LibrarySelection,
  items: readonly LibraryItem[],
): LibrarySelection {
  if (selection.groups.size === 0) return selection;
  const memberOf = new Map<number, number>();
  for (const [key, group] of selection.groups) {
    for (const id of group.videoIds) memberOf.set(id, key);
  }
  let groups: Map<number, SelectedGroup> | null = null;
  const edit = () => (groups ??= new Map(selection.groups));
  for (const item of items) {
    if (item.kind === "video") {
      const key = memberOf.get(item.video.id);
      if (key !== undefined && (groups ?? selection.groups).has(key)) edit().delete(key);
      continue;
    }
    if (item.kind === "smartFolder") continue;
    const key = item.group.id;
    const current = (groups ?? selection.groups).get(key);
    if (current === undefined) continue;
    const observed = item.group.videoIds;
    if (!observed.every((id) => selection.ids.has(id))) {
      edit().delete(key);
    } else if (
      observed.length !== current.videoIds.length ||
      observed.some((id, index) => current.videoIds[index] !== id)
    ) {
      edit().set(key, copyGroup({ id: key, videoIds: observed }));
    }
  }
  return groups === null ? selection : { ids: selection.ids, groups };
}

/** Exclude video IDs already represented by selected group targets. */
export function favoriteTargets(selection: LibrarySelection): {
  videoIds: number[];
  groupIds: number[];
} {
  const members = new Set<number>();
  const groupIds: number[] = [];
  for (const group of selection.groups.values()) {
    groupIds.push(group.id);
    for (const id of group.videoIds) members.add(id);
  }
  const videoIds = Array.from(selection.ids).filter((id) => !members.has(id));
  return { videoIds, groupIds };
}
