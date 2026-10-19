import type { TagKindRef } from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import { compareNaturalSortKeys, naturalSortKey } from "../lib/naturalSortKey";
import type { TagListSort } from "./tagListOrder";
import type { TagKindFilter } from "./tagListUrl";

export interface ListRow {
  id: number;
  name: string;
  videoCount: number;
  createdAt: string;
}

/** What paging needs of a list's conditions; local updates also need ListRowsQuery's sort. */
export interface PagedRowsQuery {
  /** Trimmed foldForMatch query; empty means unfiltered. */
  query: string;
}

export interface ListRowsQuery extends PagedRowsQuery {
  sort: TagListSort;
}

export type RowMatcher<R, Q> = (row: R, query: Q) => boolean;

export interface TagRowsQuery extends ListRowsQuery {
  tentativeOnly: boolean;
  unusedOnly: boolean;
  kind?: TagKindFilter;
}

export function matchesKind(tag: Tag, kind: TagKindFilter | undefined): boolean {
  if (kind === undefined || kind === null) return true;
  if (kind === "none") return tag.kind === null;
  return tag.kind !== null && String(tag.kind.id) === kind;
}

/** Match server name/synonym and filter rules for local mutation updates. */
export function matchesTagQuery(tag: Tag, query: TagRowsQuery): boolean {
  if (query.tentativeOnly && !tag.tentative) return false;
  if (query.unusedOnly && tag.videoCount !== 0) return false;
  if (!matchesKind(tag, query.kind)) return false;
  if (query.query === "") return true;
  return (
    foldForMatch(tag.name).includes(query.query) ||
    tag.synonyms.some((synonym) => foldForMatch(synonym).includes(query.query))
  );
}

/** Match server Unix-second ordering, using name order within the same second. */
function createdSeconds(tag: ListRow): number {
  const time = Date.parse(tag.createdAt);
  return Number.isNaN(time) ? 0 : Math.floor(time / 1000);
}

/** Match server value, natural-name-key, then ID ordering; cache name keys. */
export function compareTagsForSort<R extends ListRow>(
  a: R,
  b: R,
  sort: TagListSort,
  keys: (tag: R) => string = (tag) => naturalSortKey(tag.name),
): number {
  let byValue = 0;
  switch (sort) {
    case "countDesc":
      byValue = b.videoCount - a.videoCount;
      break;
    case "countAsc":
      byValue = a.videoCount - b.videoCount;
      break;
    case "createdDesc":
      byValue = createdSeconds(b) - createdSeconds(a);
      break;
    case "createdAsc":
      byValue = createdSeconds(a) - createdSeconds(b);
      break;
    case "name":
      break;
  }
  if (byValue !== 0) return Math.sign(byValue);
  const byKey = compareNaturalSortKeys(keys(a), keys(b));
  if (byKey !== 0) return byKey;
  return Math.sign(a.id - b.id);
}

/** Binary-search sorted rows, excluding the inserted row itself. */
export function insertionIndex<R extends ListRow>(
  rows: readonly R[],
  tag: R,
  sort: TagListSort,
): number {
  const ownKey = naturalSortKey(tag.name);
  const keys = (item: R) => (item === tag ? ownKey : naturalSortKey(item.name));
  let low = 0;
  let high = rows.length;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if (compareTagsForSort(rows[middle]!, tag, sort, keys) < 0) low = middle + 1;
    else high = middle;
  }
  return low;
}

/** Insert only within the original cursor boundary; later rows belong to future pages, while rows moved before it must be inserted locally. */
export function placeTag<R extends ListRow>(
  rows: readonly R[],
  tag: R,
  sort: TagListSort,
  boundary: R | undefined,
): R[] {
  const rest = rows.filter((item) => item.id !== tag.id);
  if (boundary !== undefined && compareTagsForSort(tag, boundary, sort) > 0) return rest;
  const index = insertionIndex(rest, tag, sort);
  return [...rest.slice(0, index), tag, ...rest.slice(index)];
}

/** Deduplicate by ID and preserve the array reference when nothing is added. */
export function appendUniqueTags<R extends ListRow>(
  rows: readonly R[],
  next: readonly R[],
): R[] {
  const seen = new Set(rows.map((tag) => tag.id));
  const added: R[] = [];
  for (const tag of next) {
    if (seen.has(tag.id)) continue;
    seen.add(tag.id);
    added.push(tag);
  }
  return added.length === 0 ? (rows as R[]) : [...rows, ...added];
}

/** Both totals include unloaded rows; nextCursor exists only when more remain. */
export interface PageRows<R extends ListRow, Q extends PagedRowsQuery> {
  rows: R[];
  total: number;
  totalAll: number;
  nextCursor: string | undefined;
  /** Preserve the final received row's original state for cursor-boundary comparisons. */
  boundary: R | undefined;
  query: Q;
}

export type TagPageRows = PageRows<Tag, TagRowsQuery>;

/** Compare before/after matches to adjust totals, including unloaded rows supplied through before. */
export function replaceTag(
  page: TagPageRows,
  before: Tag | undefined,
  after: Tag,
): TagPageRows {
  return replaceRow(page, before, after, matchesTagQuery);
}

export function replaceRow<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  before: R | undefined,
  after: R,
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  // Trust the server's match decision for rows already loaded.
  const was =
    before !== undefined &&
    (page.rows.some((tag) => tag.id === before.id) || matches(before, page.query));
  const now = matches(after, page.query);
  const rows = now
    ? placeTag(page.rows, after, page.query.sort, page.boundary)
    : page.rows.filter((tag) => tag.id !== after.id);
  return { ...page, rows, total: page.total + Number(now) - Number(was) };
}

export function removeTags(page: TagPageRows, removed: readonly Tag[]): TagPageRows {
  return removeRows(page, removed, matchesTagQuery);
}

export function removeRows<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  removed: readonly R[],
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  if (removed.length === 0) return page;
  const ids = new Set(removed.map((tag) => tag.id));
  const rows = page.rows.filter((tag) => !ids.has(tag.id));
  // Trust the server's match decision for rows already loaded.
  const loaded = page.rows.length - rows.length;
  const loadedIds = new Set(page.rows.map((tag) => tag.id));
  const others = removed.filter(
    (tag) => !loadedIds.has(tag.id) && matches(tag, page.query),
  ).length;
  return {
    ...page,
    rows,
    total: Math.max(0, page.total - loaded - others),
    totalAll: Math.max(0, page.totalAll - ids.size),
  };
}

/** Confirmation preserves sort order; remove tentative-filter mismatches in one pass. */
export function confirmTags(page: TagPageRows, ids: ReadonlySet<number>): TagPageRows {
  return updateTags(page, ids, (tag) =>
    tag.tentative ? { ...tag, tentative: false } : tag,
  );
}

/** Kind changes preserve sort order; remove rows that no longer match the kind filter. */
export function setTagsKind(
  page: TagPageRows,
  ids: ReadonlySet<number>,
  kind: TagKindRef | null,
): TagPageRows {
  return updateTags(page, ids, (tag) => ({ ...tag, kind }));
}

/** Use only for order-preserving updates; remove new mismatches and adjust totals in one pass. */
function updateTags(
  page: TagPageRows,
  ids: ReadonlySet<number>,
  update: (tag: Tag) => Tag,
): TagPageRows {
  if (ids.size === 0) return page;
  let removed = 0;
  const rows: Tag[] = [];
  for (const tag of page.rows) {
    const next = ids.has(tag.id) ? update(tag) : tag;
    if (next === tag) {
      rows.push(tag);
      continue;
    }
    if (matchesTagQuery(next, page.query)) rows.push(next);
    else removed += 1;
  }
  return { ...page, rows, total: Math.max(0, page.total - removed) };
}

export function addTag(page: TagPageRows, created: Tag): TagPageRows {
  return addRow(page, created, matchesTagQuery);
}

export function addRow<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  created: R,
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  const next = replaceRow(page, undefined, created, matches);
  return { ...next, totalAll: page.totalAll + 1 };
}
