import { RequestFailed } from "../api/client";
import {
  type Credit,
  type CreditKind,
  listCredits,
  type Organization,
  otherNames,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { readListSortPreferences } from "../preferences/tagListPreferences";
import { isTagListSort, type TagListSort } from "../tags/tagListOrder";
import type { ListRowsQuery, PageRows } from "../tags/tagPageRows";
import type { TagFieldError } from "../tags/tagNameField";
import type { RowsPage } from "../tags/useTagPage";
import { normalizeQuery } from "../videoList/listCriteria";

export const creditPageLimit = 100;

export type CreditPageRows = PageRows<Credit, ListRowsQuery>;

export function kindTexts(kind: CreditKind) {
  return kind === "person" ? t.credits.person : t.credits.organization;
}

/** Match every normalized name like the server, avoiding a refetch after mutations. */
export function matchesCreditQuery(credit: Credit, query: ListRowsQuery): boolean {
  if (query.query === "") return true;
  return credit.names.some((entry) => foldForMatch(entry.name).includes(query.query));
}

/** The first name after the display name that matches folded, unless the display name already does. */
export function matchedOtherName(credit: Credit, folded: string): string | undefined {
  if (folded === "" || foldForMatch(credit.name).includes(folded)) return undefined;
  return otherNames(credit).find((entry) => foldForMatch(entry.name).includes(folded))
    ?.name;
}

export function parentOf(credit: Credit) {
  return "parent" in credit ? credit.parent : undefined;
}

/**
 * Parent changes do not affect filtering, sorting or counts; preserve the array identity when no
 * children change.
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

export function secondLine(credit: Credit): UiText | null {
  const parts: UiText[] = [];
  const parent = parentOf(credit);
  if (parent !== undefined) parts.push(t.credits.row.partOf(parent.name));
  const others = otherNames(credit);
  if (others.length > 0) {
    const list = others
      .map((entry) =>
        entry.language === undefined
          ? entry.name
          : t.credits.row.nameWithLanguage(entry.name, entry.language),
      )
      .join(" · ");
    parts.push(t.credits.row.otherNames(list));
  }
  return parts.length === 0 ? null : t.credits.row.secondLine(parts);
}

/** Display name-field errors under the input; other failures use the general error. */
export function creditNameError(failure: unknown): TagFieldError {
  const message = errorText(failure);
  if (
    failure instanceof RequestFailed &&
    (failure.field === "name" || failure.field === "names[0].name")
  ) {
    return { kind: "taken", message };
  }
  return { kind: "other", message };
}

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

export interface CreditListCriteria {
  query: string;
  // The sorts tags share; the people grid adds the age sorts in its own criteria.
  sort: TagListSort;
}

export function sortStorageKey(kind: CreditKind): string {
  return kind === "person" ? "vv.people.v1" : "vv.organizations.v1";
}

export function readCreditSort(kind: CreditKind): TagListSort {
  return readListSortPreferences(sortStorageKey(kind)).sort;
}

/** Use the stored sort when the URL omits it or supplies an invalid value. */
export function parseCreditListCriteria(
  params: URLSearchParams,
  preferredSort: TagListSort,
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

/** Serialize q before sort and always include sort. */
export function serializeCreditListCriteria(
  criteria: CreditListCriteria,
): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  params.set("sort", criteria.sort);
  return params;
}
