import type { LibraryGroup, LibraryItem } from "../api/client";
import { itemKey } from "../api/libraryItems";

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

/** Items from one key to another in display order, both included; null when either is not listed. */
export function itemsBetween(
  items: readonly LibraryItem[],
  from: string,
  to: string,
): readonly LibraryItem[] | null {
  const keys = items.map(itemKey);
  const start = keys.indexOf(from);
  const end = keys.indexOf(to);
  if (start < 0 || end < 0) return null;
  return items.slice(Math.min(start, end), Math.max(start, end) + 1);
}

export function setItems(
  selection: LibrarySelection,
  items: readonly LibraryItem[],
  selected: boolean,
): LibrarySelection {
  return items.reduce((current, item) => {
    if (item.kind === "video") return setVideo(current, item.video.id, selected);
    if (item.kind === "group") {
      return setGroup(current, selectedGroupOf(item.group), selected);
    }
    return setSmartFolder(current, item.smartFolder.id, selected);
  }, selection);
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

/** One selected card as the bulk editor's stage shows it; a group stands for its member videos. */
export interface SelectionTarget {
  key: string;
  kind: "video" | "group" | "smartFolder";
  id: number;
  label: string;
  thumbnailUrl?: string;
  /** The videos the target sends; empty for a smart folder, whose members the server resolves. */
  videoIds: readonly number[];
}

/** The parts of a page's item that the stage reads; library, group and smart-folder items all fit. */
export interface TargetItem {
  kind: string;
  video?: { id: number; title: string; thumbnailUrl?: string };
  group?: { id: number; name: string; previews?: readonly { thumbnailUrl: string }[] };
  smartFolder?: {
    id: number;
    name: string;
    previews?: readonly { thumbnailUrl: string }[];
  };
}

/**
 * The selection as stage targets in display order: standalone videos, selected groups and smart
 * folders. A selected video that no loaded item names is appended so it is never sent unseen.
 */
export function selectionTargets(
  selection: LibrarySelection,
  items: readonly TargetItem[],
  unknownLabel: (id: number) => string,
): SelectionTarget[] {
  const { videoIds, groupIds } = favoriteTargets(selection);
  const standalone = new Set(videoIds);
  const out: SelectionTarget[] = [];
  const seen = new Set<string>();
  const push = (target: SelectionTarget) => {
    if (seen.has(target.key)) return;
    seen.add(target.key);
    out.push(target);
  };
  for (const item of items) {
    const { video, group, smartFolder } = item;
    if (video !== undefined && standalone.has(video.id)) {
      push({
        key: `v${String(video.id)}`,
        kind: "video",
        id: video.id,
        label: video.title,
        thumbnailUrl: video.thumbnailUrl,
        videoIds: [video.id],
      });
    } else if (group !== undefined && selection.groups.has(group.id)) {
      push({
        key: `g${String(group.id)}`,
        kind: "group",
        id: group.id,
        label: group.name,
        thumbnailUrl: group.previews?.[0]?.thumbnailUrl,
        videoIds: selection.groups.get(group.id)?.videoIds ?? [],
      });
    } else if (smartFolder !== undefined && selection.smartFolders.has(smartFolder.id)) {
      push({
        key: `s${String(smartFolder.id)}`,
        kind: "smartFolder",
        id: smartFolder.id,
        label: smartFolder.name,
        thumbnailUrl: smartFolder.previews?.[0]?.thumbnailUrl,
        videoIds: [],
      });
    }
  }
  for (const id of videoIds) {
    push({
      key: `v${String(id)}`,
      kind: "video",
      id,
      label: unknownLabel(id),
      videoIds: [id],
    });
  }
  for (const id of groupIds) {
    push({
      key: `g${String(id)}`,
      kind: "group",
      id,
      label: unknownLabel(id),
      videoIds: selection.groups.get(id)?.videoIds ?? [],
    });
  }
  return out;
}
