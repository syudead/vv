import type { Tag } from "../api/tags";

/**
 * 選択は読み込んだ行の id の部分集合である（specs/036-tag-admin-scale/data-model.md
 * §4「選択」）。改名中の行は選ばない（ui-design.md「Row checkbox」）。
 */

/** withoutIds は選択から ids を外した集合を返す。 */
export function withoutIds(
  current: ReadonlySet<number>,
  ids: ReadonlySet<number>,
): ReadonlySet<number> {
  const next = new Set(current);
  for (const id of ids) next.delete(id);
  return next;
}

/**
 * keepLoaded は、操作や取り直しで行から消えた id と改名中の行を選択から外す。
 * 何も外れなければ同じ集合を返す（描き直しを起こさない）。
 */
export function keepLoaded(
  current: ReadonlySet<number>,
  rows: readonly Tag[],
  renamingId: number | null,
): ReadonlySet<number> {
  if (current.size === 0) return current;
  const loaded = new Set(rows.map((tag) => tag.id));
  const next = new Set<number>();
  for (const id of current) {
    if (loaded.has(id) && id !== renamingId) next.add(id);
  }
  return next.size === current.size ? current : next;
}

/**
 * countSelectable は「読み込んだものをすべて選ぶ」の対象の数である。改名中の行は
 * 入らない（ui-design.md「Column header」）。読み込んでいないタグは選ばない（要件 10）。
 */
export function countSelectable(rows: readonly Tag[], renamingId: number | null): number {
  const renamingLoaded = renamingId !== null && rows.some((tag) => tag.id === renamingId);
  return rows.length - (renamingLoaded ? 1 : 0);
}

/** selectAllCheck は列の見出しの先頭のチェックの状態である。 */
export function selectAllCheck(
  selectedCount: number,
  selectable: number,
): boolean | "indeterminate" {
  if (selectedCount === 0) return false;
  return selectedCount >= selectable ? true : "indeterminate";
}

/**
 * selectedKinds は、選んだ行に仮のタグと確定したタグがそれぞれあるかである。選択の
 * 行は、無い種類の操作を押せなくする（ui-design.md「Enabled and disabled」）。
 */
export function selectedKinds(
  rows: readonly Tag[],
  selected: ReadonlySet<number>,
): { tentative: boolean; confirmed: boolean } {
  let tentative = false;
  let confirmed = false;
  if (selected.size === 0) return { tentative, confirmed };
  for (const tag of rows) {
    if (!selected.has(tag.id)) continue;
    if (tag.tentative) tentative = true;
    else confirmed = true;
    if (tentative && confirmed) break;
  }
  return { tentative, confirmed };
}
