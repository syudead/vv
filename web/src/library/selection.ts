import type { LibraryGroup, LibraryItem } from "../api/client";

/** Group identities used by favorite and playlist requests. */
export interface SelectedGroup {
  id: number;
  videoIds: readonly number[];
}

/** Track member video IDs for tags/visibility and group identities for favorites/playlists. Removing a member drops its group identity; remaining members become standalone selections. Smart folders are separate items that only favorites and playlists accept. */
export interface LibrarySelection {
  ids: ReadonlySet<number>;
  groups: ReadonlyMap<number, SelectedGroup>;
  smartFolders: ReadonlySet<number>;
}

export const emptySelection: LibrarySelection = {
  ids: new Set(),
  groups: new Map(),
  smartFolders: new Set(),
};

/** Videos, groups and smart folders count as one item each, like the selection bar. */
export function selectedCount(selection: LibrarySelection): number {
  return selection.ids.size + selection.smartFolders.size;
}

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
  return { ...selection, ids, groups };
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
  return { ...selection, ids };
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
  return { ...selection, ids, groups };
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
  return { ...removed, groups };
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

export function setSmartFolder(
  selection: LibrarySelection,
  id: number,
  selected: boolean,
): LibrarySelection {
  if (selection.smartFolders.has(id) === selected) return selection;
  const smartFolders = new Set(selection.smartFolders);
  if (selected) smartFolders.add(id);
  else smartFolders.delete(id);
  return { ...selection, smartFolders };
}

export function toggleSmartFolder(
  selection: LibrarySelection,
  id: number,
): LibrarySelection {
  return setSmartFolder(selection, id, !selection.smartFolders.has(id));
}

/** Drop smart folders that are no longer listed; preserve the reference when unchanged. */
export function keepSmartFolders(
  selection: LibrarySelection,
  listed: ReadonlySet<number>,
): LibrarySelection {
  const kept = [...selection.smartFolders].filter((id) => listed.has(id));
  if (kept.length === selection.smartFolders.size) return selection;
  return { ...selection, smartFolders: new Set(kept) };
}

export function fromSelectAll(
  ids: readonly number[],
  groups: readonly SelectedGroup[],
  smartFolderIds: readonly number[] = [],
): LibrarySelection {
  const selected = new Map<number, SelectedGroup>();
  for (const group of groups) selected.set(group.id, copyGroup(group));
  return { ids: new Set(ids), groups: selected, smartFolders: new Set(smartFolderIds) };
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
  return groups === null ? selection : { ...selection, groups };
}

export function sameSelection(a: LibrarySelection, b: LibrarySelection): boolean {
  if (
    a.ids.size !== b.ids.size ||
    a.groups.size !== b.groups.size ||
    a.smartFolders.size !== b.smartFolders.size
  ) {
    return false;
  }
  for (const id of a.ids) if (!b.ids.has(id)) return false;
  for (const id of a.smartFolders) if (!b.smartFolders.has(id)) return false;
  for (const key of a.groups.keys()) if (!b.groups.has(key)) return false;
  return true;
}

/** Exclude video IDs already represented by selected group targets. */
export function favoriteTargets(selection: LibrarySelection): {
  videoIds: number[];
  groupIds: number[];
  smartFolderIds: number[];
} {
  const members = new Set<number>();
  const groupIds: number[] = [];
  for (const group of selection.groups.values()) {
    groupIds.push(group.id);
    for (const id of group.videoIds) members.add(id);
  }
  const videoIds = Array.from(selection.ids).filter((id) => !members.has(id));
  return { videoIds, groupIds, smartFolderIds: Array.from(selection.smartFolders) };
}
