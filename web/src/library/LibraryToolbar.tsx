import { LayoutGrid, List, SlidersHorizontal } from "lucide-react";
import type { RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { cn } from "../lib/cn";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import Button from "../ui/Button";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import SegmentedControl from "../ui/SegmentedControl";
import Tooltip from "../ui/Tooltip";
import FilterMenu from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { CompactSortControls, SortMenu } from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

const viewOptions = [
  { value: "grid", label: "グリッド", icon: <LayoutGrid /> },
  { value: "list", label: "リスト", icon: <List /> },
] as const;

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
  canClear,
  onClear,
  view,
  onViewChange,
  zoom,
  onZoomChange,
}: LibraryToolbarProps) {
  return (
    <div className="flex min-w-0 flex-1 items-center justify-center gap-1.5">
      <SearchBox
        query={query}
        onCommit={onQueryCommit}
        inputRef={searchRef}
        className="min-w-20 flex-1 sm:min-w-40 sm:max-w-md"
      />

      <FilterMenu
        watch={watch}
        onWatchChange={onWatchChange}
        playable={playable}
        onPlayableChange={onPlayableChange}
        canClear={canClear}
        onClear={onClear}
      />

      <div className="hidden md:block">
        <SortMenu sort={sort} onSortChange={onSortChange} onShuffle={onShuffle} />
      </div>

      <div className="hidden lg:block">
        <SegmentedControl
          label="表示形式"
          value={view}
          onValueChange={onViewChange}
          options={viewOptions}
        />
      </div>

      {view === "grid" && (
        <Tooltip content="カードの大きさ">
          <ZoomSlider
            zoom={zoom}
            onZoomChange={onZoomChange}
            className="hidden w-24 xl:flex"
          />
        </Tooltip>
      )}

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="secondary"
            aria-label="表示と並び順"
            className={cn("px-2.5", view === "grid" ? "xl:hidden" : "lg:hidden")}
          >
            <SlidersHorizontal />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-80">
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
              <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
                表示形式
              </legend>
              <SegmentedControl
                label="表示形式（コンパクト）"
                value={view}
                onValueChange={onViewChange}
                options={viewOptions}
              />
            </fieldset>

            {view === "grid" && (
              <fieldset className="xl:hidden">
                <legend className="mb-1 text-xs font-semibold text-fg-muted uppercase">
                  カードの大きさ
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
