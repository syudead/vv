import { listTagPage, type Tag, tagPageLimit } from "../api/tags";
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

export function useTagPage(onFirstPage: () => void) {
  return usePagedRows<Tag, TagRowsQuery>(
    (query, search, cursor, signal) => fetchTagPage(query, search, cursor, signal),
    initialTagQuery,
    onFirstPage,
  );
}
