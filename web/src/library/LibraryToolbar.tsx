import { LayoutGrid, List, SlidersHorizontal } from "lucide-react";
import type { RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { Button } from "../ui/button";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { ToggleGroup, ToggleGroupItem } from "../ui/toggle-group";
import Tooltip from "../ui/Tooltip";
import FilterMenu from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { CompactSortControls, SortMenu } from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

/** viewOptions は表示形式の選択肢である。文言は描画のたびにカタログから引く。 */
function viewOptions() {
  return [
    { value: "grid", label: t.library.view.grid, icon: <LayoutGrid /> },
    { value: "list", label: t.library.view.list, icon: <List /> },
  ] as const;
}

/** ViewToggle は表示形式（格子・一覧）の排他の切り替えである。 */
function ViewToggle({
  label,
  view,
  onViewChange,
}: {
  label: UiText;
  view: ViewMode;
  onViewChange: (value: ViewMode) => void;
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label={label}
      value={view}
      onValueChange={(next) => {
        if (next !== "") onViewChange(next as ViewMode);
      }}
    >
      {viewOptions().map((option) => (
        <Tooltip key={option.value} content={option.label}>
          <ToggleGroupItem value={option.value} aria-label={option.label}>
            {option.icon}
          </ToggleGroupItem>
        </Tooltip>
      ))}
    </ToggleGroup>
  );
}

export interface LibraryToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  canClear: boolean;
  onClear: () => void;
  view: ViewMode;
  onViewChange: (value: ViewMode) => void;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/**
 * LibraryToolbar はライブラリ操作をトップバー内の 1 行に集める。並びは Tab の順で、
 * 検索欄 → 絞り込み → 並べ替え → 向き（並べ直す）→ 表示形式・大きさ・まとめ
 * （ui-design.md「Toolbar」）。
 */
export default function LibraryToolbar({
  query,
  onQueryCommit,
  searchRef,
  sort,
  onSortChange,
  onShuffle,
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
  view,
  onViewChange,
  zoom,
  onZoomChange,
}: LibraryToolbarProps) {
  return (
    <div className="flex min-w-0 flex-1 items-center justify-center gap-2">
      <SearchBox
        query={query}
        onCommit={onQueryCommit}
        inputRef={searchRef}
        className="min-w-search-min flex-1 sm:max-w-md sm:min-w-search-min-sm"
      />

      <FilterMenu
        watch={watch}
        onWatchChange={onWatchChange}
        playable={playable}
        onPlayableChange={onPlayableChange}
        favorite={favorite}
        onFavoriteChange={onFavoriteChange}
        canClear={canClear}
        onClear={onClear}
      />

      <div className="hidden md:block">
        <SortMenu sort={sort} onSortChange={onSortChange} onShuffle={onShuffle} />
      </div>

      <div className="hidden lg:block">
        <ViewToggle
          label={t.library.view.label}
          view={view}
          onViewChange={onViewChange}
        />
      </div>

      {view === "grid" && (
        <Tooltip content={t.list.cardSize}>
          <ZoomSlider
            zoom={zoom}
            onZoomChange={onZoomChange}
            className="hidden w-zoom xl:flex"
          />
        </Tooltip>
      )}

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={t.list.viewAndSort}
            className={cn(view === "grid" ? "xl:hidden" : "lg:hidden")}
          >
            <SlidersHorizontal />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-popover-wide">
          <div className="space-y-4">
            <div className="md:hidden">
              <CompactSortControls
                name="compact-sort"
                sort={sort}
                onSortChange={onSortChange}
                onShuffle={onShuffle}
              />
            </div>

            <fieldset className="lg:hidden">
              <legend className="mb-2 text-xs font-semibold text-muted-foreground uppercase">
                {t.library.view.label}
              </legend>
              <ViewToggle
                label={t.library.view.compact}
                view={view}
                onViewChange={onViewChange}
              />
            </fieldset>

            {view === "grid" && (
              <fieldset className="hidden sm:block xl:hidden">
                <legend className="mb-1 text-xs font-semibold text-muted-foreground uppercase">
                  {t.list.cardSize}
                </legend>
                <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />
              </fieldset>
            )}
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
