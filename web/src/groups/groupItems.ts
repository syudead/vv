import type { GroupItem } from "../api/groups";
import {
  emptySelection,
  type LibrarySelection,
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
 * keepListedGroups は、読み直した項目にもう無い子グループを選んだグループから外す。
 * 選んだ動画の id は削らない（読んでいないページの選択を残す。ライブラリと同じ）。
 */
export function keepListedGroups(
  selection: LibrarySelection,
  items: readonly GroupItem[],
): LibrarySelection {
  if (selection.groups.size === 0) return selection;
  const listed = new Set(
    items.flatMap((item) =>
      item.kind === "group" && item.group !== undefined ? [item.group.id] : [],
    ),
  );
  const groups = new Map(selection.groups);
  for (const key of groups.keys()) if (!listed.has(key)) groups.delete(key);
  return groups.size === selection.groups.size
    ? selection
    : { ids: selection.ids, groups };
}
