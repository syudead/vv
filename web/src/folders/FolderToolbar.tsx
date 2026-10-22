import type { RefObject } from "react";

import type { VideoSort } from "../api/client";
import { t, type UiText } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { Toolbar } from "../ui/patterns/toolbar";
import FilterMenu from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { CompactSortControls, SortMenu } from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

export interface FolderToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchLabel: UiText;
  searchPlaceholder: UiText;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  /** Whether Clear conditions has any active query or filter to remove. */
  canClear: boolean;
  onClear: () => void;
  /**
   * Disable filtering and sorting at the root without a query; keep search and display controls
   * enabled.
   */
  disabled?: boolean;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

export default function FolderToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchLabel,
  searchPlaceholder,
  sort,
  onSortChange,
  onShuffle,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
  disabled,
  zoom,
  onZoomChange,
}: FolderToolbarProps) {
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
          className="w-full"
          commitOn="enter"
        />
      }
      view={[
        {
          id: "sort",
          label: t.list.sort.heading,
          inlineFrom: "md",
          control: (
            <SortMenu
              sort={sort}
              onSortChange={onSortChange}
              onShuffle={onShuffle}
              disabled={disabled}
            />
          ),
          compact: (
            <CompactSortControls
              name="folder-compact-sort"
              sort={sort}
              onSortChange={onSortChange}
              onShuffle={onShuffle}
              disabled={disabled}
            />
          ),
          compactLabelled: true,
        },
        {
          id: "size",
          label: t.list.cardSize,
          inlineFrom: "xl",
          hideBelowSm: true,
          control: (
            <ZoomSlider
              zoom={zoom}
              onZoomChange={onZoomChange}
              className="w-zoom"
              tooltip
            />
          ),
          compact: (
            <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />
          ),
        },
      ]}
      viewLabel={t.list.viewAndSort}
    >
      <FilterMenu
        favorite={favorite}
        onApply={onFavoriteChange}
        canClear={canClear}
        onClear={onClear}
        disabled={disabled}
      />
    </Toolbar>
  );
}
