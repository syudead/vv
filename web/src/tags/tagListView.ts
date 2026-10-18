import { t, type UiText } from "../i18n";
import type { ListRow, ListRowsQuery, PageRows, TagPageRows } from "./tagPageRows";
import { moreIdle, type MoreState } from "./useTagPage";

/** tags means globally empty; other variants mean no current matches. */
export type TagListEmpty =
  | "tags"
  | "noUnused"
  | "noUnusedMatch"
  | "noTentative"
  | "noTentativeMatch"
  | "noKind"
  | "noKindMatch"
  | "noMatch";

type ViewQuery = ListRowsQuery & {
  tentativeOnly?: boolean;
  unusedOnly?: boolean;
  kind?: string | null;
};

function filtered({ tentativeOnly = false, unusedOnly = false, kind = null }: ViewQuery) {
  return tentativeOnly || unusedOnly || kind !== null;
}

function filteredEmpty(query: ViewQuery): TagListEmpty {
  const { tentativeOnly = false, unusedOnly = false, kind = null } = query;
  const searching = query.query !== "";
  if (kind !== null) {
    return searching || tentativeOnly || unusedOnly ? "noKindMatch" : "noKind";
  }
  if (unusedOnly) return searching ? "noUnusedMatch" : "noUnused";
  if (tentativeOnly) return searching ? "noTentativeMatch" : "noTentative";
  return "noMatch";
}

export interface TagListViewInput {
  page: PageRows<ListRow, ViewQuery> | undefined;
  loadError: UiText | null;
  more: MoreState;
  /** Includes pinned renaming rows. */
  visibleCount: number;
  creating: boolean;
}

export interface TagListView {
  empty: TagListEmpty | null;
  showRows: boolean;
  staleList: boolean;
  tail: MoreState;
  noTags: boolean;
}

/** Derive empty states from received criteria; an empty loaded page with a cursor still shows pagination. */
export function tagListView({
  page,
  loadError,
  more,
  visibleCount,
  creating,
}: TagListViewInput): TagListView {
  const staleList = page !== undefined && loadError !== null;
  const tail = page === undefined || staleList ? moreIdle : more;
  const noTags = page === undefined || page.totalAll === 0;
  if (page === undefined) {
    return { empty: null, showRows: false, staleList, tail, noTags };
  }
  const emptyTags = page.totalAll === 0 && !creating && !filtered(page.query);
  const moreToShow = page.nextCursor !== undefined && page.total > 0;
  const showRows = (visibleCount > 0 || creating || moreToShow) && !emptyTags;
  const nothingShown = visibleCount === 0 && !creating && !emptyTags && !moreToShow;
  let empty: TagListEmpty | null = null;
  if (emptyTags) empty = "tags";
  else if (nothingShown) empty = filteredEmpty(page.query);
  return { empty, showRows, staleList, tail, noTags };
}

/** Use server totals, excluding pagination boundaries and pinned renaming rows. */
export function tagCountText(page: TagPageRows | undefined): UiText {
  if (page === undefined) return t.tags.loading;
  return page.query.query !== "" || filtered(page.query)
    ? t.tags.filteredCount(page.total, page.totalAll)
    : t.tags.count(page.totalAll);
}
