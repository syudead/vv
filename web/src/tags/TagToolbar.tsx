import type { Ref, RefObject } from "react";

import type { TagKind } from "../api/tagKinds";
import type { TagListSort } from "../api/tags";
import { t } from "../i18n";
import { AdminListToolbar } from "../recordList/AdminListToolbar";
import { FilterCheckbox, FilterPopover } from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import TagKindSelect from "./TagKindSelect";
import type { TagKindFilter } from "./tagListUrl";

export interface TagToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchDisabled: boolean;
  tentativeOnly: boolean;
  onTentativeOnlyChange: (value: boolean) => void;
  unusedOnly: boolean;
  onUnusedOnlyChange: (value: boolean) => void;
  kind: TagKindFilter;
  kinds: readonly TagKind[] | undefined;
  onKindChange: (kind: TagKindFilter) => void;
  onClearFilters: () => void;
  filterDisabled: boolean;
  filterRef: Ref<HTMLButtonElement>;
  sort: TagListSort;
  onSortChange: (sort: TagListSort) => void;
  sortDisabled: boolean;
}

export default function TagToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchDisabled,
  tentativeOnly,
  onTentativeOnlyChange,
  unusedOnly,
  onUnusedOnlyChange,
  kind,
  kinds,
  onKindChange,
  onClearFilters,
  filterDisabled,
  filterRef,
  sort,
  onSortChange,
  sortDisabled,
}: TagToolbarProps) {
  const filterCount =
    (tentativeOnly ? 1 : 0) + (unusedOnly ? 1 : 0) + (kind !== null ? 1 : 0);

  return (
    <AdminListToolbar
      query={query}
      onQueryCommit={onQueryCommit}
      searchRef={searchRef}
      searchDisabled={searchDisabled}
      searchLabel={t.tags.search.label}
      searchPlaceholder={t.tags.search.placeholder}
      sort={sort}
      onSortChange={onSortChange}
      sortDisabled={sortDisabled}
      compactSortName="tag-compact-sort"
    >
      <FilterPopover
        count={filterCount}
        disabled={filterDisabled}
        canClear={filterCount > 0}
        onClear={onClearFilters}
        triggerRef={filterRef}
      >
        <div className="flex flex-col gap-3">
          <FilterCheckbox
            checked={tentativeOnly}
            onCheckedChange={onTentativeOnlyChange}
            label={t.tags.tentativeOnly}
            hint={t.tags.tentativeOnlyHint}
          />
          <FilterCheckbox
            checked={unusedOnly}
            onCheckedChange={onUnusedOnlyChange}
            label={t.tags.unusedOnly}
            hint={t.tags.unusedOnlyHint}
          />
          <TagKindSelect kind={kind} kinds={kinds} onKindChange={onKindChange} />
        </div>
      </FilterPopover>
    </AdminListToolbar>
  );
}
