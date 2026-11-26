import { listTagPage, maxTagPageLimit, type Tag, tagPageLimit } from "../api/tags";
import { type RowsPage, usePagedRows } from "../recordList/usePagedRows";
import type { TagRowsQuery } from "./tagPageRows";

export type TagPageQuery = TagRowsQuery & { search: string };

const initialTagQuery: TagPageQuery = {
  query: "",
  search: "",
  tentativeOnly: false,
  unusedOnly: false,
  kind: null,
  sort: "name",
};

/** One page of the list for its criteria; search is the raw text the folded query came from. */
export function fetchTagPage(
  query: TagRowsQuery,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
  limit = tagPageLimit,
): Promise<RowsPage<Tag>> {
  return listTagPage(
    {
      q: query.query === "" ? undefined : search.trim(),
      tentative: query.tentativeOnly,
      unused: query.unusedOnly,
      kind: query.kind ?? undefined,
      sort: query.sort,
      cursor,
      limit,
    },
    signal,
  );
}

export interface TagPagesRead {
  rows: Tag[];
  total: number;
  totalAll: number;
}

/** The list's rows changed while its pages were read one after another. */
export class TagListMoved extends Error {}

/**
 * Read from the cursor in the server's largest pages until `wanted` rows or the list's end; a
 * `totalAll` other than `expectAll` means the list changed under the read and throws `TagListMoved`.
 */
export async function readTagPages(
  query: TagRowsQuery,
  search: string,
  from: { cursor: string | undefined; wanted: number; expectAll?: number },
  signal: AbortSignal,
): Promise<TagPagesRead> {
  const rows: Tag[] = [];
  let cursor = from.cursor;
  for (;;) {
    const limit = Math.min(maxTagPageLimit, Math.max(1, from.wanted - rows.length));
    const page = await fetchTagPage(query, search, cursor, signal, limit);
    if (from.expectAll !== undefined && page.totalAll !== from.expectAll) {
      throw new TagListMoved();
    }
    rows.push(...page.items);
    cursor = page.nextCursor;
    if (cursor === undefined || rows.length >= from.wanted) {
      return {
        rows: rows.slice(0, from.wanted),
        total: page.total,
        totalAll: page.totalAll,
      };
    }
  }
}

export function useTagPage(onFirstPage: () => void) {
  return usePagedRows<Tag, TagRowsQuery>(
    (query, search, cursor, signal) => fetchTagPage(query, search, cursor, signal),
    initialTagQuery,
    onFirstPage,
  );
}
