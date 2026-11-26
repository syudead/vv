import type { Tag } from "../api/tags";

export function withoutIds(
  current: ReadonlySet<number>,
  ids: ReadonlySet<number>,
): ReadonlySet<number> {
  const next = new Set(current);
  for (const id of ids) next.delete(id);
  return next;
}

/** Keep only loaded cards selected; preserve the set reference when none is removed. */
export function keepLoaded(
  current: ReadonlySet<number>,
  rows: readonly Tag[],
): ReadonlySet<number> {
  if (current.size === 0) return current;
  const loaded = new Set(rows.map((tag) => tag.id));
  const next = new Set<number>();
  for (const id of current) {
    if (loaded.has(id)) next.add(id);
  }
  return next.size === current.size ? current : next;
}

/** The ids of the cards from one to the other in list order, both included; empty when either is not loaded. */
export function idsBetween(rows: readonly Tag[], from: number, to: number): number[] {
  const a = rows.findIndex((tag) => tag.id === from);
  const b = rows.findIndex((tag) => tag.id === to);
  if (a < 0 || b < 0) return [];
  return rows.slice(Math.min(a, b), Math.max(a, b) + 1).map((tag) => tag.id);
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
