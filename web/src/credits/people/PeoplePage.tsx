import { Cake, CakeSlice, ScanFace, Users } from "lucide-react";
import { useState } from "react";

import type { Person } from "../../api/credits";
import { t } from "../../i18n";
import { EmptyState } from "../../ui/patterns/empty-state";
import type { SortOption } from "../../videoList/SortControls";
import { CreditGridPage } from "../CreditGridPage";
import { personPath } from "../person/personPath";
import { useLocalToday } from "../person/useLocalToday";
import {
  type AgeSort,
  fetchPeoplePage,
  parsePeopleCriteria,
  type PeopleCriteria,
  peopleFilterCount,
  readPeopleSort,
  serializePeopleCriteria,
} from "./peopleCriteria";
import NameFilterSelect from "./NameFilterSelect";
import { PersonCard } from "./PersonCard";

const grid = t.credits.peopleGrid;

const ageSorts: readonly SortOption<AgeSort>[] = [
  { value: "ageAsc", label: grid.youngestFirst, icon: CakeSlice },
  { value: "ageDesc", label: grid.oldestFirst, icon: Cake },
];

const initialQuery: PeopleCriteria & { search: string } = {
  query: "",
  search: "",
  sort: "name",
  favorite: false,
  named: "any",
};

function unnamedEmpty(query: PeopleCriteria) {
  if (query.named !== "unnamed") return null;
  return (
    <EmptyState
      icon={<ScanFace aria-hidden="true" />}
      title={grid.noUnnamed}
      description={grid.noUnnamedDescription}
    />
  );
}

/** `/people`: a grid of face cards paged 100 at a time; each card opens the person page. */
export default function PeoplePage() {
  const today = useLocalToday();
  const [preferredSort] = useState(readPeopleSort);
  return (
    <CreditGridPage<Person, PeopleCriteria, AgeSort>
      kind="person"
      fetchPage={fetchPeoplePage}
      initialQuery={initialQuery}
      parse={(params) => parsePeopleCriteria(params, preferredSort)}
      serialize={serializePeopleCriteria}
      extraSorts={ageSorts}
      filterCount={peopleFilterCount}
      noFilters={{ named: "any" }}
      filters={(criteria, change) => (
        <NameFilterSelect
          value={criteria.named}
          onChange={(next) => change({ named: next })}
        />
      )}
      filteredEmpty={unnamedEmpty}
      cardSize="face"
      emptyIcon={<Users aria-hidden="true" />}
      noFavorites={grid.noFavorites}
      newDialog={grid.newDialog}
      pagePath={personPath}
      renderCard={(person, onToggleFavorite) => (
        <PersonCard
          key={person.id}
          person={person}
          today={today}
          onToggleFavorite={onToggleFavorite}
        />
      )}
    />
  );
}
