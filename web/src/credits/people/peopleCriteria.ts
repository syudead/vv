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

export interface PeopleCriteria {
  query: string;
  sort: CreditSort;
  favorite: boolean;
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
    },
    hasExplicitSort,
  };
}

/** Serialize q, then sort, then favorite only when on. */
export function serializePeopleCriteria(criteria: PeopleCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  params.set("sort", criteria.sort);
  if (criteria.favorite) params.set("favorite", "true");
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
      cursor,
      limit: creditPageLimit,
    },
    signal,
  );
}

export type PeopleBody =
  "loading" | "failed" | "noPeople" | "noMatches" | "noFavorites" | "cards";

/** What the grid's body shows; the empty kinds follow the received page's conditions. */
export function peopleBody(
  page: PeoplePageRows | undefined,
  loadError: UiText | null,
  firstPending: boolean,
): PeopleBody {
  if (loadError !== null) return "failed";
  if (page === undefined || firstPending) return "loading";
  if (page.rows.length > 0 || page.nextCursor !== undefined) return "cards";
  if (page.totalAll === 0) return "noPeople";
  if (page.query.query !== "") return "noMatches";
  return page.query.favorite ? "noFavorites" : "noPeople";
}

export function peopleCountText(page: PeoplePageRows | undefined): UiText | null {
  if (page === undefined) return null;
  const texts = t.credits.person;
  return page.query.query === "" && !page.query.favorite
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
