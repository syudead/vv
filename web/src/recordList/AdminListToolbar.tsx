import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarPlus,
  Hash,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode, RefObject } from "react";

import { isTagListSort, type TagListSort } from "../api/tags";
import { t, type UiText } from "../i18n";
import { Toolbar } from "../ui/patterns/toolbar";
import IconToggleGroup from "../videoList/IconToggleGroup";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import {
  CompactSortView,
  SortMenuView,
  type SortOption,
} from "../videoList/SortControls";
import {
  tagSortDirection,
  tagSortKind,
  tagSortKinds,
  type TagSortDirection,
  type TagSortKind,
  withTagSortDirection,
} from "./tagListOrder";

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

export interface AdminListToolbarProps<E extends string = never> {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchDisabled: boolean;
  searchLabel: UiText;
  searchPlaceholder: UiText;
  sort: TagListSort | E;
  onSortChange: (sort: TagListSort | E) => void;
  /** Sorts after the shared ones, each one option without a direction toggle. */
  extraSorts?: readonly SortOption<E>[];
  sortDisabled: boolean;
  /** Unique radio-group name per screen. */
  compactSortName: string;
  children?: ReactNode;
}

export function AdminListToolbar<E extends string = never>({
  query,
  onQueryCommit,
  searchRef,
  searchDisabled,
  searchLabel,
  searchPlaceholder,
  sort: current,
  onSortChange,
  extraSorts = [],
  sortDisabled,
  compactSortName,
  children,
}: AdminListToolbarProps<E>) {
  const extra = extraSorts.find((option) => option.value === current);
  // An extra sort has no direction, as the name order has none.
  const sort: TagListSort = extra === undefined ? (current as TagListSort) : "name";
  const kind = tagSortKind(sort);
  const direction = tagSortDirection(sort);
  const options: readonly SortOption<TagListSort | E>[] = [
    ...tagSortOptions(),
    ...extraSorts,
  ];
  const value = extra?.value ?? activeOption(sort);
  const label = extra?.label ?? t.tags.sort.kinds[kind];
  // Choosing the active kind again keeps its direction.
  const choose = (next: TagListSort | E) => {
    if (next === value) return;
    if (extra !== undefined || !isTagListSort(next) || tagSortKind(next) !== kind) {
      onSortChange(next);
    }
  };

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
              value={value}
              options={options}
              onValueChange={choose}
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
              label={label}
              currentLabel={t.tags.sort.current(label)}
              heading={t.tags.sort.heading}
              value={value}
              options={options}
              onValueChange={choose}
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
