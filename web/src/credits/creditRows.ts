import { RequestFailed } from "../api/client";
import {
  type Credit,
  type CreditKind,
  type CreditSort,
  listCredits,
  type Organization,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { readListSortPreferences } from "../preferences/tagListPreferences";
import { isTagListSort } from "../tags/tagListOrder";
import type { ListRowsQuery, PageRows } from "../tags/tagPageRows";
import type { TagFieldError } from "../tags/tagNameField";
import type { RowsPage } from "../tags/useTagPage";
import { normalizeQuery } from "../videoList/listCriteria";

/** creditPageLimit は一覧の 1 ページの件数である（contracts/screen-api.md「People and organizations」）。 */
export const creditPageLimit = 100;

/** CreditPageRows は人物・団体の管理画面の読み込んだ行と件数である。 */
export type CreditPageRows = PageRows<Credit, ListRowsQuery>;

/** kindTexts は種類で言い分ける文言である。 */
export function kindTexts(kind: CreditKind) {
  return kind === "person" ? t.credits.person : t.credits.organization;
}

/**
 * matchesCreditQuery は、人物・団体が検索に合うかである。サーバーの `GET /api/people` と
 * 同じく、検索語の照合形が名前か別名の照合形に部分一致する。操作のあとの 1 行を往復せずに
 * 判定するのに使う。
 */
export function matchesCreditQuery(credit: Credit, query: ListRowsQuery): boolean {
  if (query.query === "") return true;
  return (
    foldForMatch(credit.name).includes(query.query) ||
    credit.aliases.some((alias) => foldForMatch(alias.name).includes(query.query))
  );
}

/** parentOf は団体の親を返す。人物と根の団体は undefined。 */
export function parentOf(credit: Credit) {
  return "parent" in credit ? credit.parent : undefined;
}

/**
 * reparentRows は、団体 `fromId` の改名・削除・統合のあと、読み込んだ行のうちその団体を親に
 * 持つ子の `parent` を書き換える。`to` は新しい親（改名ならその団体、統合なら統合先）で、
 * undefined なら親を外す（削除はサーバーが子の親を外す。data-model.md「Entities」）。親は
 * 並び順と検索に関わらないので、行の位置と件数は変えない。子が無ければ同じ配列を返す。
 */
export function reparentRows(
  rows: Credit[],
  fromId: number,
  to: Organization["parent"],
): Credit[] {
  if (!rows.some((row) => parentOf(row)?.id === fromId)) return rows;
  return rows.map((row) => {
    if (parentOf(row)?.id !== fromId) return row;
    const next: Organization = { ...row };
    if (to === undefined) delete next.parent;
    else next.parent = to;
    return next;
  });
}

/**
 * secondLine は行の名前の下の 1 行である。親（団体）を先に、別名を言語つきで並べる。
 * どちらも無ければ null（ui-design.md「People and organizations screens」の Words）。
 */
export function secondLine(credit: Credit): UiText | null {
  const parts: UiText[] = [];
  const parent = parentOf(credit);
  if (parent !== undefined) parts.push(t.credits.row.partOf(parent.name));
  if (credit.aliases.length > 0) {
    const list = credit.aliases
      .map((alias) =>
        alias.language === undefined
          ? alias.name
          : t.credits.row.aliasWithLanguage(alias.name, alias.language),
      )
      .join(" · ");
    parts.push(t.credits.row.aliases(list));
  }
  return parts.length === 0 ? null : t.credits.row.secondLine(parts);
}

/**
 * creditNameError は作成・改名の失敗の分け方である。名前に向いた理由（`field: name`）は
 * 入力の下の小さな理由（`taken` の形）、それ以外は一般の失敗（`other`）にする。
 */
export function creditNameError(failure: unknown): TagFieldError {
  const message = errorText(failure);
  if (failure instanceof RequestFailed && failure.field === "name") {
    return { kind: "taken", message };
  }
  return { kind: "other", message };
}

/**
 * creditCountText は見出しの件数である。検索している間は `total` of `totalAll`。先頭の
 * ページを待つ間は出さない（ui-design.md「States」の Loading）。
 */
export function creditCountText(
  kind: CreditKind,
  page: CreditPageRows | undefined,
): UiText | null {
  if (page === undefined) return null;
  const texts = kindTexts(kind);
  return page.query.query === ""
    ? texts.count(page.totalAll)
    : texts.filteredCount(page.total, page.totalAll);
}

/** fetchCreditPage は種類の一覧の 1 ページを読む読み方を返す（`usePagedRows`）。 */
export function fetchCreditPage(kind: CreditKind) {
  return (
    query: ListRowsQuery,
    search: string,
    cursor: string | undefined,
    signal: AbortSignal,
  ): Promise<RowsPage<Credit>> =>
    listCredits(
      kind,
      {
        q: query.query === "" ? undefined : search.trim(),
        sort: query.sort,
        cursor,
        limit: creditPageLimit,
      },
      signal,
    );
}

/** CreditListCriteria は管理画面の URL に載せる条件である（`q` と `sort`）。 */
export interface CreditListCriteria {
  query: string;
  sort: CreditSort;
}

/** sortStorageKey は種類ごとの、端末に残す並び順の鍵である。 */
export function sortStorageKey(kind: CreditKind): string {
  return kind === "person" ? "vv.people.v1" : "vv.organizations.v1";
}

/** readCreditSort は端末に残した並び順を読む。無ければ名前の順。 */
export function readCreditSort(kind: CreditKind): CreditSort {
  return readListSortPreferences(sortStorageKey(kind)).sort;
}

/**
 * parseCreditListCriteria は URL のクエリを条件にする。解釈できない値は既定とし、sort が
 * 無いか解釈できなければ端末に残した並び順を使う（タグの画面の `parseTagListCriteria`）。
 */
export function parseCreditListCriteria(
  params: URLSearchParams,
  preferredSort: CreditSort,
): { criteria: CreditListCriteria; hasExplicitSort: boolean } {
  const rawSort = params.get("sort");
  const hasExplicitSort = isTagListSort(rawSort);
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      sort: hasExplicitSort ? rawSort : preferredSort,
    },
    hasExplicitSort,
  };
}

/** serializeCreditListCriteria は条件を URL のクエリにする。順は q・sort で、sort は書く。 */
export function serializeCreditListCriteria(
  criteria: CreditListCriteria,
): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  params.set("sort", criteria.sort);
  return params;
}
