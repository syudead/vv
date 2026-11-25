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

function fetchTagPage(
  query: TagRowsQuery,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
): Promise<RowsPage<Tag>> {
  return listTagPage(
    {
      q: query.query === "" ? undefined : search.trim(),
      tentative: query.tentativeOnly,
      unused: query.unusedOnly,
      kind: query.kind ?? undefined,
      sort: query.sort,
      cursor,
      limit: tagPageLimit,
    },
    signal,
  );
}

export function useTagPage(onFirstPage: () => void) {
  return usePagedRows<Tag, TagRowsQuery>(fetchTagPage, initialTagQuery, onFirstPage);
}
