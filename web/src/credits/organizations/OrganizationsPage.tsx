import { Building2 } from "lucide-react";
import { useState } from "react";

import {
  type CreditSort,
  listCredits,
  type Organization,
  type OrganizationList,
} from "../../api/credits";
import { t } from "../../i18n";
import { readListSortPreferences } from "../../preferences/tagListPreferences";
import { isTagListSort } from "../../tags/tagListOrder";
import { CreditGridPage } from "../CreditGridPage";
import {
  type CreditGridCriteria,
  parseCreditGridCriteria,
  serializeCreditGridCriteria,
} from "../creditGrid";
import { creditPageLimit, sortStorageKey } from "../creditRows";
import { organizationPath } from "../organization/organizationPath";
import { OrganizationCard } from "./OrganizationCard";

const grid = t.credits.organizationGrid;

const initialQuery: CreditGridCriteria & { search: string } = {
  query: "",
  search: "",
  sort: "name",
  favorite: false,
};

function readOrganizationSort(): CreditSort {
  return readListSortPreferences(sortStorageKey("organization")).sort;
}

function fetchOrganizationsPage(
  query: CreditGridCriteria,
  search: string,
  cursor: string | undefined,
  signal: AbortSignal,
): Promise<OrganizationList> {
  return listCredits(
    "organization",
    {
      q: query.query === "" ? undefined : search.trim(),
      sort: query.sort,
      favorite: query.favorite,
      cursor,
      limit: creditPageLimit,
    },
    signal,
  ) as Promise<OrganizationList>;
}

function favoriteCount(criteria: CreditGridCriteria) {
  return criteria.favorite ? 1 : 0;
}

/** `/organizations`: a grid of logo cards paged 100 at a time; each card opens the organization page. */
export default function OrganizationsPage() {
  const [preferredSort] = useState(readOrganizationSort);
  return (
    <CreditGridPage<Organization, CreditGridCriteria>
      kind="organization"
      fetchPage={fetchOrganizationsPage}
      initialQuery={initialQuery}
      parse={(params) => parseCreditGridCriteria(params, preferredSort, isTagListSort)}
      serialize={serializeCreditGridCriteria}
      filterCount={favoriteCount}
      noFilters={{}}
      cardSize="logo"
      emptyIcon={<Building2 aria-hidden="true" />}
      noFavorites={grid.noFavorites}
      newDialog={grid.newDialog}
      pagePath={organizationPath}
      renderCard={(organization, onToggleFavorite) => (
        <OrganizationCard
          key={organization.id}
          organization={organization}
          onToggleFavorite={onToggleFavorite}
        />
      )}
    />
  );
}
