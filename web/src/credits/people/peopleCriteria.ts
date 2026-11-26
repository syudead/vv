import { type CreditSort, listPeople, type Person } from "../../api/credits";
import { t, type UiText } from "../../i18n";
import { readStoredSort } from "../../preferences/tagListPreferences";
import { isTagListSort } from "../../api/tags";
import type { RowsPage } from "../../recordList/usePagedRows";
import {
  type CreditGridCriteria,
  parseCreditGridCriteria,
  serializeCreditGridCriteria,
} from "../creditGrid";
import { creditPageLimit, sortStorageKey } from "../creditRows";
import { ageRange } from "../person/personFacts";

export type AgeSort = Extract<CreditSort, "ageAsc" | "ageDesc">;

/** Which people the Name filter keeps: provisional people have no name. */
export type NameFilter = "any" | "named" | "unnamed";

export const nameFilters: readonly NameFilter[] = ["any", "named", "unnamed"];

/** The grid's conditions; on a received page, query is folded and trimmed as the server matches it. */
export interface PeopleCriteria extends CreditGridCriteria {
  named: NameFilter;
}

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
  const { criteria, hasExplicitSort } = parseCreditGridCriteria(
    params,
    preferredSort,
    isPeopleSort,
  );
  return {
    criteria: { ...criteria, named: namedFromParam(params.get("named")) },
    hasExplicitSort,
  };
}

function namedFromParam(value: string | null): NameFilter {
  if (value === "1") return "named";
  if (value === "0") return "unnamed";
  return "any";
}

/** Serialize the grid's conditions, then the Name filter only when applied. */
export function serializePeopleCriteria(criteria: PeopleCriteria): URLSearchParams {
  const params = serializeCreditGridCriteria(criteria);
  if (criteria.named !== "any")
    params.set("named", criteria.named === "named" ? "1" : "0");
  return params;
}

export function fetchPeoplePage(
  query: PeopleCriteria,
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

/** The number of applied toolbar filters; the search is shown in its input instead. */
export function peopleFilterCount(criteria: Pick<PeopleCriteria, "favorite" | "named">) {
  return (criteria.favorite ? 1 : 0) + (criteria.named === "any" ? 0 : 1);
}

/** The age on the viewer's date (a range at partial precision), then the video count. */
export function personCardMeta(person: Person, today: Date): UiText {
  const { birthDate, deathDate } = person.profile;
  const range = birthDate === undefined ? null : ageRange(birthDate, deathDate, today);
  const age =
    range === null ? null : t.credits.personPage.ageRange(range.low, range.high);
  return t.credits.peopleGrid.cardMeta(age, person.videoCount);
}
