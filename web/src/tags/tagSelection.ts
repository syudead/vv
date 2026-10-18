import type { Tag } from "../api/tags";

/** Select loaded rows only, excluding rows being renamed. */

export function withoutIds(
  current: ReadonlySet<number>,
  ids: ReadonlySet<number>,
): ReadonlySet<number> {
  const next = new Set(current);
  for (const id of ids) next.delete(id);
  return next;
}

/** Preserve the set reference when no IDs are removed. */
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

export function countSelectable(rows: readonly Tag[], renamingId: number | null): number {
  const renamingLoaded = renamingId !== null && rows.some((tag) => tag.id === renamingId);
  return rows.length - (renamingLoaded ? 1 : 0);
}

export function selectAllCheck(
  selectedCount: number,
  selectable: number,
): boolean | "indeterminate" {
  if (selectedCount === 0) return false;
  return selectedCount >= selectable ? true : "indeterminate";
}

export interface SelectionSummary {
  tentative: boolean;
  confirmed: boolean;
  /** Shared kind ID or none; undefined for mixed or empty selections. */
  kind: string | undefined;
}

function kindValue(tag: Tag): string {
  return tag.kind === null ? "none" : String(tag.kind.id);
}

export function selectedKinds(
  rows: readonly Tag[],
  selected: ReadonlySet<number>,
): SelectionSummary {
  let tentative = false;
  let confirmed = false;
  let kind: string | undefined;
  let mixed = false;
  if (selected.size === 0) return { tentative, confirmed, kind };
  for (const tag of rows) {
    if (!selected.has(tag.id)) continue;
    if (tag.tentative) tentative = true;
    else confirmed = true;
    const value = kindValue(tag);
    if (kind === undefined) kind = value;
    else if (kind !== value) mixed = true;
  }
  return { tentative, confirmed, kind: mixed ? undefined : kind };
}
