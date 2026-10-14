import type { FolderRef, LibraryGroup, LibraryItem } from "../api/client";
import { groupFolder } from "../api/libraryItems";

/**
 * SelectedGroup はグループのカードのチェック（または「すべて選択」の応答の `groups`）で
 * グループとして選んだものである（specs/035-favorites/research.md R-7）。グループの id で
 * 指す（specs/048-groups/contracts/screen-api.md）。`folder` は自動グループのフォルダで、
 * お気に入りがまだフォルダで送るために持つ。手で作ったグループと、「すべて選択」で読み込んで
 * いないグループでは無い。`unread` は後者で、送る前に `GET /api/groups/{id}` でフォルダを
 * 引く（groupFolders.ts の withGroupFolders）。
 */
export interface SelectedGroup {
  id: number;
  folder?: FolderRef;
  unread?: true;
  videoIds: readonly number[];
}

/**
 * LibrarySelection はライブラリの選択である。`ids` は選んだ動画の id の集合（グループは
 * メンバーを数える）で、タグ・公開・束ねる操作はこれを送る。`groups` はグループとして
 * 選んだもの（グループの id ごと）で、一括のお気に入りだけがこれを `folders` として送る。
 * グループのメンバーを 1 本でも外したグループは `groups` から外れ、残ったメンバーは
 * 動画として扱われる。
 */
export interface LibrarySelection {
  ids: ReadonlySet<number>;
  groups: ReadonlyMap<number, SelectedGroup>;
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
  groups.set(group.id, copyGroup(group));
  return { ids, groups };
}

/** copyGroup は選んだグループを、呼び出し側の値と切り離して覚える。 */
function copyGroup(group: SelectedGroup): SelectedGroup {
  const copy: SelectedGroup = { id: group.id, videoIds: group.videoIds };
  if (group.folder !== undefined) {
    copy.folder = { rootId: group.folder.rootId, path: group.folder.path };
  }
  if (group.unread === true) copy.unread = true;
  return copy;
}

/** selectedGroupOf はグループの項目をグループとして選ぶときの値にする。 */
export function selectedGroupOf(group: LibraryGroup): SelectedGroup {
  return copyGroup({
    id: group.id,
    folder: groupFolder(group),
    videoIds: group.videoIds,
  });
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

/**
 * fromSelectAll は「すべて選択」の応答（`ids` と `groups`）を選択にする。応答の `groups` は
 * フォルダを持たないので、読み込んだ項目 `items` にあるグループのフォルダで補う。読み込んで
 * いないグループは `unread` にする。
 */
export function fromSelectAll(
  ids: readonly number[],
  groups: readonly { id: number; videoIds: readonly number[] }[],
  items: readonly LibraryItem[] = [],
): LibrarySelection {
  const loaded = new Map<number, LibraryGroup>();
  for (const item of items) {
    if (item.kind === "group") loaded.set(item.group.id, item.group);
  }
  const selected = new Map<number, SelectedGroup>();
  for (const group of groups) {
    const shown = loaded.get(group.id);
    selected.set(
      group.id,
      shown === undefined
        ? { id: group.id, unread: true, videoIds: group.videoIds }
        : copyGroup({
            id: group.id,
            folder: groupFolder(shown),
            videoIds: group.videoIds,
          }),
    );
  }
  return { ids: new Set(ids), groups: selected };
}

/**
 * reconcileGroups は、読み込んだ項目から分かったグループの変化を選んだグループに映す。
 * 選択を解かずに一覧を取り直す（取り込みの完了など）と、選んだグループがいちばん上の項目で
 * なくなったり、メンバーが変わったりする。そのまま `folders` に載せると、サーバーはグループで
 * ないフォルダを飛ばし、メンバーは `videoIds` から除かれているので何も付かない。
 *
 * - 選んだグループのメンバーが動画の項目として載っていれば、そのグループは今の一覧では
 *   グループの項目でない（同じグループのカードとそのメンバーの項目は同時に出ない。
 *   specs/027-partial-group-search/contracts/library-api.md §1）。グループから外し、
 *   メンバーは選んだ動画として残す。
 * - 同じ id のグループの項目が載っていれば、そのメンバーとフォルダで覚え直す。全メンバーが
 *   選択に入っていなければ（メンバーが増えた）グループから外す（removeIds と同じ考え方）。
 *
 * id の集合は変えない（読み込んでいない項目の選択を削らない）。変化が無ければ同じ選択を返す。
 */
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
    const key = item.group.id;
    const current = (groups ?? selection.groups).get(key);
    if (current === undefined) continue;
    const observed = item.group.videoIds;
    const folder = groupFolder(item.group);
    if (!observed.every((id) => selection.ids.has(id))) {
      edit().delete(key);
    } else if (
      current.unread === true ||
      observed.length !== current.videoIds.length ||
      observed.some((id, index) => current.videoIds[index] !== id) ||
      (folder === undefined) !== (current.folder === undefined) ||
      (folder !== undefined &&
        (folder.rootId !== current.folder?.rootId || folder.path !== current.folder.path))
    ) {
      edit().set(key, copyGroup({ id: key, folder, videoIds: observed }));
    }
  }
  return groups === null ? selection : { ids: selection.ids, groups };
}

/** sameSelection は動画の id の集合と選んだグループの両方が同じとき true を返す。 */
export function sameSelection(a: LibrarySelection, b: LibrarySelection): boolean {
  if (a.ids.size !== b.ids.size || a.groups.size !== b.groups.size) return false;
  for (const id of a.ids) if (!b.ids.has(id)) return false;
  for (const key of a.groups.keys()) if (!b.groups.has(key)) return false;
  return true;
}

/**
 * favoriteTargets は一括のお気に入りで送るものである（R-7）。`folders` はフォルダの分かる
 * 選んだグループ、`unreadGroupIds` は「すべて選択」で読み込んでいないグループで、送る前に
 * フォルダを引く（withGroupFolders）。`videoIds` は選んだ id のうち選んだグループのメンバーで
 * ないもの。フォルダの無い（手で作った）グループは送らない（お気に入りはまだフォルダで送る）。
 */
export function favoriteTargets(selection: LibrarySelection): {
  videoIds: number[];
  folders: FolderRef[];
  unreadGroupIds: number[];
} {
  const members = new Set<number>();
  const folders: FolderRef[] = [];
  const unreadGroupIds: number[] = [];
  for (const group of selection.groups.values()) {
    if (group.folder !== undefined) folders.push(group.folder);
    else if (group.unread === true) unreadGroupIds.push(group.id);
    for (const id of group.videoIds) members.add(id);
  }
  const videoIds = Array.from(selection.ids).filter((id) => !members.has(id));
  return { videoIds, folders, unreadGroupIds };
}
