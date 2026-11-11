import { LayoutGrid, List } from "lucide-react";
import type { ReactNode, RefObject } from "react";

import type { VideoSort } from "../api/client";
import { t, type UiText } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { Toolbar, type ToolbarViewControl } from "../ui/patterns/toolbar";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import FilterMenu from "../videoList/FilterMenu";
import IconToggleGroup from "../videoList/IconToggleGroup";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox, { type FileSearchField } from "../videoList/SearchBox";
import type { SearchSuggestions } from "../videoList/SearchSuggestions";
import {
  CompactSortControls,
  SortMenu,
  type NativeOrder,
} from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

function viewOptions() {
  return [
    {
      value: "grid",
      label: t.library.view.grid,
      icon: <LayoutGrid aria-hidden="true" />,
    },
    { value: "list", label: t.library.view.list, icon: <List aria-hidden="true" /> },
  ] as const;
}

/** A disabled control with the reason on hover; the wrapper takes the pointer the disabled control ignores. */
function Locked({
  reason,
  children,
}: {
  reason: UiText | undefined;
  children: ReactNode;
}) {
  if (reason === undefined) return children;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span data-slot="locked-control" className="inline-flex">
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent>{reason}</TooltipContent>
    </Tooltip>
  );
}

export function viewControls(
  view: ViewMode,
  onViewChange: (value: ViewMode) => void,
  zoom: Zoom,
  onZoomChange: (value: Zoom) => void,
  lockedReason?: UiText,
): ToolbarViewControl[] {
  const locked = lockedReason !== undefined;
  const controls: ToolbarViewControl[] = [
    {
      id: "view",
      label: t.library.view.label,
      inlineFrom: "lg",
      control: (
        <Locked reason={lockedReason}>
          <IconToggleGroup
            label={t.library.view.label}
            value={view}
            onValueChange={onViewChange}
            options={viewOptions()}
            disabled={locked}
          />
        </Locked>
      ),
      compact: (
        <Locked reason={lockedReason}>
          <IconToggleGroup
            label={t.library.view.compact}
            value={view}
            onValueChange={onViewChange}
            options={viewOptions()}
            disabled={locked}
          />
        </Locked>
      ),
    },
  ];
  // An image search draws cards, so the size stays while the list view is chosen.
  if (view === "grid" || locked) {
    controls.push({
      id: "size",
      label: t.list.cardSize,
      inlineFrom: "xl",
      hideBelowSm: true,
      control: (
        <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-zoom" tooltip />
      ),
      compact: <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />,
    });
  }
  return controls;
}

export interface LibraryToolbarProps {
  nativeOrder?: NativeOrder;
  /** Search field name and placeholder; omitted uses the library's. */
  searchLabel?: UiText;
  searchPlaceholder?: UiText;
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  favorite: boolean;
  /** Applies the filter popover's drafts: favorites here, the page's own fields with it. */
  onFilterApply: (favorite: boolean) => void;
  canClear: boolean;
  onClear: () => void;
  view: ViewMode;
  onViewChange: (value: ViewMode) => void;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
  /** Owner fields drawn before `Favorites only`. */
  extraFilter?: ReactNode;
  extraFilterCount?: number;
  onFilterOpen?: () => void;
  suggestions?: SearchSuggestions;
  /** The owner's library field searches by a file too. */
  fileSearch?: FileSearchField;
  /** Disables filter, sort and view with this reason, as an image search does; zoom stays. */
  lockedReason?: UiText;
}

export default function LibraryToolbar({
  query,
  onQueryCommit,
  searchRef,
  sort,
  onSortChange,
  onShuffle,
  favorite,
  onFilterApply,
  canClear,
  onClear,
  view,
  onViewChange,
  zoom,
  onZoomChange,
  extraFilter,
  extraFilterCount,
  onFilterOpen,
  nativeOrder,
  suggestions,
  searchLabel,
  searchPlaceholder,
  fileSearch,
  lockedReason,
}: LibraryToolbarProps) {
  const locked = lockedReason !== undefined;
  const controls: ToolbarViewControl[] = [
    {
      id: "sort",
      label: t.list.sort.heading,
      inlineFrom: "md",
      control: (
        <Locked reason={lockedReason}>
          <SortMenu
            sort={sort}
            onSortChange={onSortChange}
            onShuffle={onShuffle}
            nativeOrder={nativeOrder}
            disabled={locked}
          />
        </Locked>
      ),
      compact: (
        <Locked reason={lockedReason}>
          <CompactSortControls
            nativeOrder={nativeOrder}
            name="compact-sort"
            sort={sort}
            onSortChange={onSortChange}
            onShuffle={onShuffle}
            disabled={locked}
          />
        </Locked>
      ),
      compactLabelled: true,
    },
    ...viewControls(view, onViewChange, zoom, onZoomChange, lockedReason),
  ];

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
          suggestions={suggestions}
          commitOn="enter"
          fileSearch={fileSearch}
        />
      }
      view={controls}
      viewLabel={t.list.viewAndSort}
    >
      <Locked reason={lockedReason}>
        <FilterMenu
          favorite={favorite}
          onApply={onFilterApply}
          canClear={canClear}
          onClear={onClear}
          extraCount={extraFilterCount}
          onOpen={onFilterOpen}
          disabled={locked}
        >
          {extraFilter}
        </FilterMenu>
      </Locked>
    </Toolbar>
  );
}
