import { type CreditSort, listPeople, type Person } from "../../api/credits";
import { t, type UiText } from "../../i18n";
import { readStoredSort } from "../../preferences/tagListPreferences";
import { isTagListSort } from "../../tags/tagListOrder";
import type { PageRows } from "../../tags/tagPageRows";
import type { RowsPage } from "../../tags/useTagPage";
import { normalizeQuery } from "../../videoList/listCriteria";
import { creditPageLimit, sortStorageKey } from "../creditRows";
import { ageRange } from "../person/personFacts";

export type AgeSort = Extract<CreditSort, "ageAsc" | "ageDesc">;

/** Which people the Name filter keeps: provisional people have no name. */
export type NameFilter = "any" | "named" | "unnamed";

export const nameFilters: readonly NameFilter[] = ["any", "named", "unnamed"];

export interface PeopleCriteria {
  query: string;
  sort: CreditSort;
  favorite: boolean;
  named: NameFilter;
}

/** The conditions of a received page; query is folded and trimmed as the server matches it. */
export type PeopleRowsQuery = PeopleCriteria;

export type PeoplePageRows = PageRows<Person, PeopleRowsQuery>;

export function isPeopleSort(value: unknown): value is CreditSort {
  return isTagListSort(value) || value === "ageAsc" || value === "ageDesc";
}

export function readPeopleSort(): CreditSort {
  return readStoredSort(sortStorageKey("person"), isPeopleSort, "name");
}

/** Use the stored sort when the URL omits it or supplies an invalid value. */
export function parsePeopleCriteria(
  params: URLSearchParams,
  preferredSort: CreditSort,
): { criteria: PeopleCriteria; hasExplicitSort: boolean } {
  const rawSort = params.get("sort");
  const hasExplicitSort = isPeopleSort(rawSort);
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      sort: hasExplicitSort ? rawSort : preferredSort,
      favorite: params.get("favorite") === "true",
      named: namedFromParam(params.get("named")),
    },
    hasExplicitSort,
  };
}

function namedFromParam(value: string | null): NameFilter {
  if (value === "1") return "named";
  if (value === "0") return "unnamed";
  return "any";
}

/** Serialize q, then sort, then favorite and the Name filter only when applied. */
export function serializePeopleCriteria(criteria: PeopleCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  params.set("sort", criteria.sort);
  if (criteria.favorite) params.set("favorite", "true");
  if (criteria.named !== "any")
    params.set("named", criteria.named === "named" ? "1" : "0");
  return params;
}

export function fetchPeoplePage(
  query: PeopleRowsQuery,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
): Promise<RowsPage<Person>> {
  return listPeople(
    {
      q: query.query === "" ? undefined : search.trim(),
      sort: query.sort,
      favorite: query.favorite,
      provisional: query.named === "any" ? undefined : query.named === "unnamed",
      cursor,
      limit: creditPageLimit,
    },
    signal,
  );
}

export type PeopleBody =
  "loading" | "failed" | "noPeople" | "noMatches" | "noFavorites" | "noUnnamed" | "cards";

/** The number of applied toolbar filters; the search is shown in its input instead. */
export function peopleFilterCount(criteria: Pick<PeopleCriteria, "favorite" | "named">) {
  return (criteria.favorite ? 1 : 0) + (criteria.named === "any" ? 0 : 1);
}

/**
 * What the grid's body shows; the empty kinds follow the received page's conditions.
 * A failed refresh keeps the received page, which `isStalePeople` marks with a notice.
 */
export function peopleBody(
  page: PeoplePageRows | undefined,
  loadError: UiText | null,
  firstPending: boolean,
): PeopleBody {
  if (page === undefined) return loadError === null ? "loading" : "failed";
  if (firstPending && loadError === null) return "loading";
  if (page.rows.length > 0 || page.nextCursor !== undefined) return "cards";
  if (page.totalAll === 0) return "noPeople";
  if (page.query.query !== "") return "noMatches";
  if (page.query.favorite) return "noFavorites";
  return page.query.named === "unnamed" ? "noUnnamed" : "noPeople";
}

export function isStalePeople(
  page: PeoplePageRows | undefined,
  loadError: UiText | null,
): boolean {
  return page !== undefined && loadError !== null;
}

export function peopleCountText(page: PeoplePageRows | undefined): UiText | null {
  if (page === undefined) return null;
  const texts = t.credits.person;
  return page.query.query === "" && peopleFilterCount(page.query) === 0
    ? texts.count(page.totalAll)
    : texts.filteredCount(page.total, page.totalAll);
}

/** The age on the viewer's date (a range at partial precision), then the video count. */
export function personCardMeta(person: Person, today: Date): UiText {
  const { birthDate, deathDate } = person.profile;
  const range = birthDate === undefined ? null : ageRange(birthDate, deathDate, today);
  const age =
    range === null ? null : t.credits.personPage.ageRange(range.low, range.high);
  return t.credits.peopleGrid.cardMeta(age, person.videoCount);
}
