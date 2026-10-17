import { t, type UiText } from "../i18n";
import type { ListRow, ListRowsQuery, PageRows, TagPageRows } from "./tagPageRows";
import { moreIdle, type MoreState } from "./useTagPage";

/**
 * TagListEmpty は一覧の代わりに出す空の状態である。
 * `tags` はタグが 1 つも無い、ほかは今の条件に合う行が無い。
 */
export type TagListEmpty =
  | "tags"
  | "noUnused"
  | "noUnusedMatch"
  | "noTentative"
  | "noTentativeMatch"
  | "noKind"
  | "noKindMatch"
  | "noMatch";

/** ViewQuery は空の状態を選ぶのに読む、届いた行の条件である。 */
type ViewQuery = ListRowsQuery & {
  tentativeOnly?: boolean;
  unusedOnly?: boolean;
  kind?: string | null;
};

/** filtered は「Filter」の絞り込み（検索は数えない）がどれか効いているかである。 */
function filtered({ tentativeOnly = false, unusedOnly = false, kind = null }: ViewQuery) {
  return tentativeOnly || unusedOnly || kind !== null;
}

/**
 * filteredEmpty は、タグはあるが今の条件に合う行が無いときの空の状態である。種類の
 * 絞り込みが効いていれば、種類だけなら `noKind`、検索かほかの絞り込みと重なれば
 * `noKindMatch`。
 */
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

/**
 * TagListViewInput は一覧の見え方を決める状態である。人物・団体の画面も、絞り込みの無い
 * 条件（`tentativeOnly`・`unusedOnly` を持たない）で同じ規則を使う。そのときの空の状態は
 * `tags`（1 件も無い）か `noMatch` である。
 */
export interface TagListViewInput {
  page: PageRows<ListRow, ViewQuery> | undefined;
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
   * 検索・並び順は押せない。
   */
  noTags: boolean;
}

/**
 * tagListView は一覧の領域の見え方を決める。空の状態は、入力中の条件ではなく届いた
 * 行の条件（`page.query`）で選ぶので、条件を変えて先頭のページを待つ間は前の状態が
 * 残る。読み込んだ行が無くても続きがあれば、空の状態を
 * 出さずに末尾の続きの状態を出す。
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
  const emptyTags = page.totalAll === 0 && !creating && !filtered(page.query);
  const moreToShow = page.nextCursor !== undefined && page.total > 0;
  const showRows = (visibleCount > 0 || creating || moreToShow) && !emptyTags;
  const nothingShown = visibleCount === 0 && !creating && !emptyTags && !moreToShow;
  let empty: TagListEmpty | null = null;
  if (emptyTags) empty = "tags";
  else if (nothingShown) empty = filteredEmpty(page.query);
  return { empty, showRows, staleList, tail, noTags };
}

/**
 * tagCountText は見出しの件数である。応答の `total`・`totalAll` で出し、読み込んだ行の
 * 数（ページの区切り）や差し込んで残した改名中の行は数えない。
 */
export function tagCountText(page: TagPageRows | undefined): UiText {
  if (page === undefined) return t.tags.loading;
  return page.query.query !== "" || filtered(page.query)
    ? t.tags.filteredCount(page.total, page.totalAll)
    : t.tags.count(page.totalAll);
}
