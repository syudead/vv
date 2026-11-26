import type { TagKindRef } from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import {
  addRow,
  type ListRowsQuery,
  type PageRows,
  removeRows,
  replaceRow,
} from "../recordList/listRows";
import type { TagKindFilter } from "./tagListUrl";

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

export type TagPageRows = PageRows<Tag, TagRowsQuery>;

/** Compare before/after matches to adjust totals, including unloaded rows supplied through before. */
export function replaceTag(
  page: TagPageRows,
  before: Tag | undefined,
  after: Tag,
): TagPageRows {
  return replaceRow(page, before, after, matchesTagQuery);
}

export function removeTags(page: TagPageRows, removed: readonly Tag[]): TagPageRows {
  return removeRows(page, removed, matchesTagQuery);
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
