import type { GroupItem } from "../api/groups";
import {
  emptySelection,
  type LibrarySelection,
  type SelectedGroup,
  selectedGroupOf,
  setGroup,
} from "../library/selection";

// グループのページが項目から決める規則（specs/048-groups/ui-design.md「Body」）。

/**
 * appendItems は読んだ続きのページを今の項目の後ろへ足す。同じ項目（id）が前のページに
 * あれば足さない（読む間に並びが変わって、ページの境目で同じ項目が二度返ったとき）。
 */
export function appendItems(
  current: readonly GroupItem[],
  page: readonly GroupItem[],
): GroupItem[] {
  const known = new Set(current.map((item) => item.id));
  return [...current, ...page.filter((item) => !known.has(item.id))];
}

/**
 * selectAllInGroup はグループのページの「Select all」の選択である。動画はグループの
 * `videoIds`（子孫の動画のすべて、読んでいないページの分も）、選んだグループはここまでに
 * 並んだ子グループ（ui-design.md「Body」）。
 */
export function selectAllInGroup(
  videoIds: readonly number[],
  items: readonly GroupItem[],
): LibrarySelection {
  let selection = emptySelection;
  for (const item of items) {
    if (item.kind === "group" && item.group !== undefined) {
      selection = setGroup(selection, selectedGroupOf(item.group), true);
    }
  }
  const ids = new Set(selection.ids);
  for (const id of videoIds) ids.add(id);
  return { ids, groups: selection.groups };
}

/**
 * reconcileGroupSelection は読み直したグループを選択に映す（ui-design.md「Edits that fail」
 * 「The selection drops the ids no longer listed」）。
 *
 * - 選んだ動画は、グループの `videoIds`（子孫の動画のすべて、読んでいないページの分も）に
 *   もう無いものを外す。読んでいないページの選択は残る。
 * - 選んだ子グループは、読み直した項目にもう無ければ外す。並んでいれば今のメンバーで
 *   覚え直し、メンバーが増えて全員が選択に入っていなければ外す（ライブラリの
 *   `reconcileGroups` と同じ考え方。外したグループのメンバーは選んだ動画として残る）。
 *
 * ライブラリの `reconcileGroups` をそのまま使わないのは、子グループのメンバーが直接の
 * 動画の項目としても並ぶ（Edge Case）ことがあり、その規則だと子グループの選択を誤って外すため。
 * 変化が無ければ同じ選択を返す。
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

/** sameIds は 2 つの id の並びが同じとき true を返す。 */
function sameIds(a: readonly number[], b: readonly number[]): boolean {
  return a.length === b.length && a.every((id, index) => b[index] === id);
}
