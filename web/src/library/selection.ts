import type { FolderRef } from "../api/client";
import { folderRefKey } from "../api/libraryItems";

/**
 * SelectedGroup はグループのカードのチェック（または「すべて選択」の応答の `groups`）で
 * グループとして選んだものである（specs/035-favorites/research.md R-7）。
 */
export interface SelectedGroup {
  folder: FolderRef;
  videoIds: readonly number[];
}

/**
 * LibrarySelection はライブラリの選択である。`ids` は選んだ動画の id の集合（グループは
 * メンバーを数える）で、タグ・公開・束ねる操作はこれを送る。`groups` はグループとして
 * 選んだもの（フォルダの鍵ごと）で、一括のお気に入りだけがこれを `folders` として送る。
 * グループのメンバーを 1 本でも外したグループは `groups` から外れ、残ったメンバーは
 * 動画として扱われる。
 */
export interface LibrarySelection {
  ids: ReadonlySet<number>;
  groups: ReadonlyMap<string, SelectedGroup>;
}

export const emptySelection: LibrarySelection = { ids: new Set(), groups: new Map() };

/** removeIds は id を選択から外し、外した id をメンバーに含むグループも外す。 */
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

/** setVideo は動画 1 本を選択に入れる・外す。 */
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

/** toggleVideo は動画 1 本の選択を切り替える。 */
export function toggleVideo(selection: LibrarySelection, id: number): LibrarySelection {
  return setVideo(selection, id, !selection.ids.has(id));
}

/**
 * setGroup はグループの全メンバーを選択に入れて、グループとして覚える。外すときは全メンバーを
 * 外す（グループも外れる）。
 */
export function setGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
  selected: boolean,
): LibrarySelection {
  if (!selected) return removeIds(selection, group.videoIds);
  const ids = new Set(selection.ids);
  for (const id of group.videoIds) ids.add(id);
  const groups = new Map(selection.groups);
  groups.set(folderRefKey(group.folder), {
    folder: { rootId: group.folder.rootId, path: group.folder.path },
    videoIds: group.videoIds,
  });
  return { ids, groups };
}

/** toggleGroup は全メンバーが選択に入っていれば外し、そうでなければグループとして選ぶ。 */
export function toggleGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
): LibrarySelection {
  const all =
    group.videoIds.length > 0 && group.videoIds.every((id) => selection.ids.has(id));
  return setGroup(selection, group, !all);
}

/** fromSelectAll は「すべて選択」の応答（`ids` と `groups`）を選択にする。 */
export function fromSelectAll(
  ids: readonly number[],
  groups: readonly SelectedGroup[],
): LibrarySelection {
  const selected = new Map<string, SelectedGroup>();
  for (const group of groups) {
    selected.set(folderRefKey(group.folder), {
      folder: { rootId: group.folder.rootId, path: group.folder.path },
      videoIds: group.videoIds,
    });
  }
  return { ids: new Set(ids), groups: selected };
}

/** sameSelection は動画の id の集合と選んだグループの両方が同じとき true を返す。 */
export function sameSelection(a: LibrarySelection, b: LibrarySelection): boolean {
  if (a.ids.size !== b.ids.size || a.groups.size !== b.groups.size) return false;
  for (const id of a.ids) if (!b.ids.has(id)) return false;
  for (const key of a.groups.keys()) if (!b.groups.has(key)) return false;
  return true;
}

/**
 * favoriteTargets は一括のお気に入りで送るものである（R-7）。`folders` は選んだグループ、
 * `videoIds` は選んだ id のうち選んだグループのメンバーでないもの。
 */
export function favoriteTargets(selection: LibrarySelection): {
  videoIds: number[];
  folders: FolderRef[];
} {
  const members = new Set<number>();
  const folders: FolderRef[] = [];
  for (const group of selection.groups.values()) {
    folders.push(group.folder);
    for (const id of group.videoIds) members.add(id);
  }
  const videoIds = Array.from(selection.ids).filter((id) => !members.has(id));
  return { videoIds, folders };
}
