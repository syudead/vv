import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarPlus,
  Hash,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode, Ref, RefObject } from "react";

import type { TagKind } from "../api/tagKinds";
import { t, type UiText } from "../i18n";
import { Toolbar } from "../ui/patterns/toolbar";
import { FilterCheckbox, FilterPopover } from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import IconToggleGroup from "../videoList/IconToggleGroup";
import {
  CompactSortView,
  SortMenuView,
  type SortOption,
} from "../videoList/SortControls";
import {
  tagSortDirection,
  tagSortKind,
  tagSortKinds,
  withTagSortDirection,
  type TagListSort,
  type TagSortDirection,
  type TagSortKind,
} from "./tagListOrder";
import TagKindSelect from "./TagKindSelect";
import type { TagKindFilter } from "./tagListUrl";

/** Tag creation differs from file creation, so use a distinct icon. */
const tagSortIcons: Record<TagSortKind, LucideIcon> = {
  name: ArrowDownAZ,
  count: Hash,
  created: CalendarPlus,
};

function tagSortOptions(): SortOption<TagListSort>[] {
  return tagSortKinds.map((option) => ({
    value: option.initial,
    label: t.tags.sort.kinds[option.kind],
    icon: tagSortIcons[option.kind],
  }));
}

function activeOption(sort: TagListSort): TagListSort {
  const kind = tagSortKind(sort);
  return tagSortKinds.find((option) => option.kind === kind)!.initial;
}

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

export interface AdminListToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchDisabled: boolean;
  searchLabel: UiText;
  searchPlaceholder: UiText;
  sort: TagListSort;
  onSortChange: (sort: TagListSort) => void;
  sortDisabled: boolean;
  /** Unique radio-group name per screen. */
  compactSortName: string;
  children?: ReactNode;
}

export function AdminListToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchDisabled,
  searchLabel,
  searchPlaceholder,
  sort,
  onSortChange,
  sortDisabled,
  compactSortName,
  children,
}: AdminListToolbarProps) {
  const kind = tagSortKind(sort);
  const direction = tagSortDirection(sort);
  const options = tagSortOptions();

  return (
    <Toolbar
      placement="topBar"
      search={
        <SearchBox
          query={query}
          onCommit={onQueryCommit}
          inputRef={searchRef}
          label={searchLabel}
          placeholder={searchPlaceholder}
          syntaxHelp={false}
          debounceMs={0}
          disabled={searchDisabled}
        />
      }
      view={[
        {
          id: "sort",
          label: t.tags.sort.heading,
          inlineFrom: "md",
          compactLabelled: true,
          compact: (
            <CompactSortView
              name={compactSortName}
              heading={t.tags.sort.heading}
              value={activeOption(sort)}
              options={options}
              onValueChange={(next) => {
                if (tagSortKind(next) !== kind) onSortChange(next);
              }}
              disabled={sortDisabled}
            >
              {direction !== undefined && (
                <IconToggleGroup<TagSortDirection>
                  label={t.tags.sort.direction}
                  value={direction}
                  onValueChange={(next) => onSortChange(withTagSortDirection(sort, next))}
                  options={[
                    {
                      value: "desc",
                      label:
                        kind === "count"
                          ? t.tags.sort.segments.countDesc
                          : t.tags.sort.segments.createdDesc,
                      icon: <ArrowDownWideNarrow aria-hidden="true" />,
                    },
                    {
                      value: "asc",
                      label:
                        kind === "count"
                          ? t.tags.sort.segments.countAsc
                          : t.tags.sort.segments.createdAsc,
                      icon: <ArrowUpNarrowWide aria-hidden="true" />,
                    },
                  ]}
                />
              )}
            </CompactSortView>
          ),
          control: (
            <SortMenuView
              label={t.tags.sort.kinds[kind]}
              currentLabel={t.tags.sort.current(t.tags.sort.kinds[kind])}
              heading={t.tags.sort.heading}
              value={activeOption(sort)}
              options={options}
              onValueChange={(next) => {
                if (tagSortKind(next) !== kind) onSortChange(next);
              }}
              // Natural name order has no direction toggle.
              toggle={
                direction === undefined || sort === "name"
                  ? undefined
                  : {
                      label: t.tags.sort.toggle[sort],
                      icon:
                        direction === "desc" ? (
                          <ArrowDownWideNarrow aria-hidden="true" />
                        ) : (
                          <ArrowUpNarrowWide aria-hidden="true" />
                        ),
                      onClick: () =>
                        onSortChange(
                          withTagSortDirection(
                            sort,
                            direction === "asc" ? "desc" : "asc",
                          ),
                        ),
                    }
              }
              disabled={sortDisabled}
            />
          ),
        },
      ]}
      viewLabel={t.tags.sort.compact}
    >
      {children}
    </Toolbar>
  );
}
