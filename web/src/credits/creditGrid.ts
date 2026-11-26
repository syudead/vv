import type { CreditKind, CreditSort } from "../api/credits";
import type { UiText } from "../i18n";
import type { ListRow, PageRows } from "../tags/tagPageRows";
import { normalizeQuery } from "../videoList/listCriteria";
import { kindTexts } from "./creditRows";

/** The conditions every credit grid keeps in its URL; a kind may add filters of its own. */
export interface CreditGridCriteria {
  query: string;
  sort: CreditSort;
  favorite: boolean;
}

/** Use the stored sort when the URL omits it or supplies an invalid value. */
export function parseCreditGridCriteria(
  params: URLSearchParams,
  preferredSort: CreditSort,
  isSort: (value: unknown) => value is CreditSort,
): { criteria: CreditGridCriteria; hasExplicitSort: boolean } {
  const rawSort = params.get("sort");
  const hasExplicitSort = isSort(rawSort);
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      sort: hasExplicitSort ? rawSort : preferredSort,
      favorite: params.get("favorite") === "true",
    },
    hasExplicitSort,
  };
}

/** Serialize q, then sort, then favorite only when applied. */
export function serializeCreditGridCriteria(
  criteria: CreditGridCriteria,
): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  params.set("sort", criteria.sort);
  if (criteria.favorite) params.set("favorite", "true");
  return params;
}

/** `filtered` is an empty page under a kind's own filter, which the kind may describe. */
export type CreditGridBody =
  "loading" | "failed" | "none" | "noMatches" | "noFavorites" | "filtered" | "cards";

/**
 * What the grid's body shows; the empty kinds follow the received page's conditions.
 * A failed refresh keeps the received page, which the grid marks with a notice.
 */
export function creditGridBody<R extends ListRow, Q extends CreditGridCriteria>(
  page: PageRows<R, Q> | undefined,
  loadError: UiText | null,
  firstPending: boolean,
): CreditGridBody {
  if (page === undefined) return loadError === null ? "loading" : "failed";
  if (firstPending && loadError === null) return "loading";
  if (page.rows.length > 0 || page.nextCursor !== undefined) return "cards";
  if (page.totalAll === 0) return "none";
  if (page.query.query !== "") return "noMatches";
  return page.query.favorite ? "noFavorites" : "filtered";
}

/** The total alone while nothing narrows the page, otherwise the matches out of the total. */
export function creditGridCountText<R extends ListRow, Q extends CreditGridCriteria>(
  kind: CreditKind,
  page: PageRows<R, Q> | undefined,
  filterCount: (query: Q) => number,
): UiText | null {
  if (page === undefined) return null;
  const texts = kindTexts(kind);
  return page.query.query === "" && filterCount(page.query) === 0
    ? texts.count(page.totalAll)
    : texts.filteredCount(page.total, page.totalAll);
}
