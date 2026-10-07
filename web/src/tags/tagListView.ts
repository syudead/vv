import { t, type UiText } from "../i18n";
import type { TagPageRows } from "./tagPageRows";
import { moreIdle, type MoreState } from "./useTagPage";

/**
 * TagListEmpty は一覧の代わりに出す空の状態である（ui-design.md「States」）。
 * `tags` はタグが 1 つも無い、ほかは今の条件に合う行が無い。
 */
export type TagListEmpty =
  "tags" | "noUnused" | "noUnusedMatch" | "noTentative" | "noTentativeMatch" | "noMatch";

/** TagListViewInput は一覧の見え方を決める状態である。 */
export interface TagListViewInput {
  page: TagPageRows | undefined;
  loadError: UiText | null;
  more: MoreState;
  /** 並べる行の数（差し込んで残した改名中の行を含む）。 */
  visibleCount: number;
  creating: boolean;
}

/** TagListView は一覧の領域に何を出すかである。 */
export interface TagListView {
  empty: TagListEmpty | null;
  /** 表（列の見出し・作成の行・行）を出すか。 */
  showRows: boolean;
  /** 一覧を持ったまま先頭のページを読めなかったか（「Stale list」）。 */
  staleList: boolean;
  /** 一覧の末尾の続きの状態で、「Stale list」の間は出さない。 */
  tail: MoreState;
  /**
   * 先頭のページをまだ受けていないか、タグが 1 つも無いか。押していない絞り込みと
   * 検索・並び順は押せない（ui-design.md「Top bar」）。
   */
  noTags: boolean;
}

/**
 * tagListView は一覧の領域の見え方を決める。空の状態は、入力中の条件ではなく届いた
 * 行の条件（`page.query`）で選ぶので、条件を変えて先頭のページを待つ間は前の状態が
 * 残る（ui-design.md「States」）。読み込んだ行が無くても続きがあれば、空の状態を
 * 出さずに末尾の続きの状態を出す（ui-design.md「Loading more」）。
 */
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
  const { tentativeOnly, unusedOnly, query } = page.query;
  const searching = query !== "";
  const emptyTags = page.totalAll === 0 && !creating && !tentativeOnly && !unusedOnly;
  const moreToShow = page.nextCursor !== undefined && page.total > 0;
  const showRows = (visibleCount > 0 || creating || moreToShow) && !emptyTags;
  const nothingShown = visibleCount === 0 && !creating && !emptyTags && !moreToShow;
  let empty: TagListEmpty | null = null;
  if (emptyTags) empty = "tags";
  else if (nothingShown) {
    if (unusedOnly) empty = searching ? "noUnusedMatch" : "noUnused";
    else if (tentativeOnly) empty = searching ? "noTentativeMatch" : "noTentative";
    else empty = "noMatch";
  }
  return { empty, showRows, staleList, tail, noTags };
}

/**
 * tagCountText は見出しの件数である。応答の `total`・`totalAll` で出し、読み込んだ行の
 * 数（ページの区切り）や差し込んで残した改名中の行は数えない（ui-design.md「Header」）。
 */
export function tagCountText(page: TagPageRows | undefined): UiText {
  if (page === undefined) return t.tags.loading;
  const { query, tentativeOnly, unusedOnly } = page.query;
  return query !== "" || tentativeOnly || unusedOnly
    ? t.tags.filteredCount(page.total, page.totalAll)
    : t.tags.count(page.totalAll);
}
