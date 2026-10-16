import type { LibraryGroup, LibraryItem } from "../api/client";

/**
 * SelectedGroup はグループのカードのチェック（または「すべて選択」の応答の `groups`）で
 * グループとして選んだものである（specs/035-favorites/research.md R-7）。グループの id で
 * 指し、お気に入りとプレイリストへの追加もその id を `groupIds` で送る
 * （specs/048-groups/research.md R-10）。
 */
export interface SelectedGroup {
  id: number;
  videoIds: readonly number[];
}

/**
 * LibrarySelection はライブラリの選択である。`ids` は選んだ動画の id の集合（グループは
 * メンバーを数える）で、タグ・公開・束ねる操作はこれを送る。`groups` はグループとして
 * 選んだもの（グループの id ごと）で、一括のお気に入りとプレイリストへの追加がこれを
 * `groupIds` として送る。
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
 * setGroup はグループの全メンバーを選択に入れて、グループとして覚える。外すときはグループを外し、
 * メンバーのうち、選んだままの別のグループのメンバーでもあるものを残して外す（deselectGroup）。
 */
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

/**
 * deselectGroup はグループを選択から外す。メンバーを 2 つのグループが共有するとき、選んだままの
 * 別のグループのメンバーは残すので、そのグループも選んだままになる
 * （specs/048-groups/ui-design.md「Selection with shared members」）。
 */
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

/** copyGroup は選んだグループを、呼び出し側の値と切り離して覚える。 */
function copyGroup(group: SelectedGroup): SelectedGroup {
  return { id: group.id, videoIds: group.videoIds };
}

/** selectedGroupOf はグループの項目をグループとして選ぶときの値にする。 */
export function selectedGroupOf(group: LibraryGroup): SelectedGroup {
  return copyGroup(group);
}

/**
 * isGroupSelected はグループのカードのチェックの状態である。グループとして選んだもの（`groups`）
 * にあるときだけ true で、メンバーの動画が選択に入っているかでは決めない。メンバーを共有する
 * 別のグループを選んだままこのグループを外すと、メンバーは選択に残るが、このカードは外れて
 * 見え、もう一度押せば選べる（specs/048-groups/ui-design.md「Selection with shared members」）。
 */
export function isGroupSelected(selection: LibrarySelection, groupId: number): boolean {
  return selection.groups.has(groupId);
}

/** toggleGroup はグループとして選んでいれば外し、そうでなければグループとして選ぶ。 */
export function toggleGroup(
  selection: LibrarySelection,
  group: SelectedGroup,
): LibrarySelection {
  return setGroup(selection, group, !isGroupSelected(selection, group.id));
}

/** fromSelectAll は「すべて選択」の応答（`ids` と `groups`）を選択にする。 */
export function fromSelectAll(
  ids: readonly number[],
  groups: readonly SelectedGroup[],
): LibrarySelection {
  const selected = new Map<number, SelectedGroup>();
  for (const group of groups) selected.set(group.id, copyGroup(group));
  return { ids: new Set(ids), groups: selected };
}

/**
 * reconcileGroups は、読み込んだ項目から分かったグループの変化を選んだグループに映す。
 * 選択を解かずに一覧を取り直す（取り込みの完了など）と、選んだグループがいちばん上の項目で
 * なくなったり、メンバーが変わったりする。そのまま `groupIds` に載せると、メンバーは `videoIds`
 * から除かれているので、選んだ動画でなくグループに付いてしまう。
 *
 * - 選んだグループのメンバーが動画の項目として載っていれば、そのグループは今の一覧では
 *   グループの項目でない（同じグループのカードとそのメンバーの項目は同時に出ない。
 *   specs/027-partial-group-search/contracts/library-api.md §1）。グループから外し、
 *   メンバーは選んだ動画として残す。
 * - 同じ id のグループの項目が載っていれば、そのメンバーで覚え直す。全メンバーが
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

/** sameSelection は動画の id の集合と選んだグループの両方が同じとき true を返す。 */
export function sameSelection(a: LibrarySelection, b: LibrarySelection): boolean {
  if (a.ids.size !== b.ids.size || a.groups.size !== b.groups.size) return false;
  for (const id of a.ids) if (!b.ids.has(id)) return false;
  for (const key of a.groups.keys()) if (!b.groups.has(key)) return false;
  return true;
}

/**
 * favoriteTargets は一括のお気に入りとプレイリストへの追加で送るものである（R-7）。`groupIds` は
 * 選んだグループ、`videoIds` は選んだ id のうち選んだグループのメンバーでないもの。
 */
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
