import { LayoutGrid, List } from "lucide-react";
import type { ReactNode, RefObject } from "react";

import type { VideoSort } from "../api/client";
import { t } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { Toolbar, type ToolbarViewControl } from "../ui/patterns/toolbar";
import FilterMenu from "../videoList/FilterMenu";
import IconToggleGroup from "../videoList/IconToggleGroup";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
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

export function viewControls(
  view: ViewMode,
  onViewChange: (value: ViewMode) => void,
  zoom: Zoom,
  onZoomChange: (value: Zoom) => void,
): ToolbarViewControl[] {
  const controls: ToolbarViewControl[] = [
    {
      id: "view",
      label: t.library.view.label,
      inlineFrom: "lg",
      control: (
        <IconToggleGroup
          label={t.library.view.label}
          value={view}
          onValueChange={onViewChange}
          options={viewOptions()}
        />
      ),
      compact: (
        <IconToggleGroup
          label={t.library.view.compact}
          value={view}
          onValueChange={onViewChange}
          options={viewOptions()}
        />
      ),
    },
  ];
  if (view === "grid") {
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
}: LibraryToolbarProps) {
  const controls: ToolbarViewControl[] = [
    {
      id: "sort",
      label: t.list.sort.heading,
      inlineFrom: "md",
      control: (
        <SortMenu
          sort={sort}
          onSortChange={onSortChange}
          onShuffle={onShuffle}
          nativeOrder={nativeOrder}
        />
      ),
      compact: (
        <CompactSortControls
          nativeOrder={nativeOrder}
          name="compact-sort"
          sort={sort}
          onSortChange={onSortChange}
          onShuffle={onShuffle}
        />
      ),
      compactLabelled: true,
    },
    ...viewControls(view, onViewChange, zoom, onZoomChange),
  ];

  return (
    <Toolbar
      placement="topBar"
      search={
        <SearchBox
          query={query}
          onCommit={onQueryCommit}
          inputRef={searchRef}
          suggestions={suggestions}
          commitOn="enter"
        />
      }
      view={controls}
      viewLabel={t.list.viewAndSort}
    >
      <FilterMenu
        favorite={favorite}
        onApply={onFilterApply}
        canClear={canClear}
        onClear={onClear}
        extraCount={extraFilterCount}
        onOpen={onFilterOpen}
      >
        {extraFilter}
      </FilterMenu>
    </Toolbar>
  );
}
