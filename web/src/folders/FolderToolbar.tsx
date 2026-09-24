import { SlidersHorizontal } from "lucide-react";
import type { RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { FilterMenu, ZoomSlider } from "../library/LibraryToolbar";
import type { HistoryMode } from "../library/listCriteria";
import SearchBox from "../library/SearchBox";
import { CompactSortControls, SortMenu } from "../library/SortControls";
import type { Zoom } from "../preferences/viewPreferences";
import Button from "../ui/Button";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import Tooltip from "../ui/Tooltip";

export interface FolderToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchLabel: string;
  searchPlaceholder: string;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  /** 検索語・視聴状態・再生可否のどれかが効いているか（「条件を解除」を出す）。 */
  canClear: boolean;
  onClear: () => void;
  /**
   * 最上位（`/folders`）で検索語が空のときは true。絞り込み・並べ替え・向きを
   * 無効にする（ui-design.md「Top level」）。検索欄と表示形式は無効にしない。
   */
  disabled?: boolean;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/**
 * FolderToolbar はフォルダ画面（各フォルダ・最上位）のトップバー内の操作である。
 * ライブラリのツールバーから表示形式の切り替えを除いたもので、部品と幅の境界は
 * 同じにする（ui-design.md「Toolbar」「Screen boundary」）。検索欄・絞り込み・
 * 並べ替え・向きの部品は `web/src/library/` のものを借りる（Plan の
 * Structural Decisions 9）。
 */
export default function FolderToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchLabel,
  searchPlaceholder,
  sort,
  onSortChange,
  onShuffle,
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  canClear,
  onClear,
  disabled,
  zoom,
  onZoomChange,
}: FolderToolbarProps) {
  return (
    <div className="flex min-w-0 flex-1 items-center justify-center gap-1.5">
      <SearchBox
        query={query}
        onCommit={onQueryCommit}
        inputRef={searchRef}
        label={searchLabel}
        placeholder={searchPlaceholder}
        className="min-w-20 flex-1 sm:min-w-40 sm:max-w-md"
      />

      <FilterMenu
        watch={watch}
        onWatchChange={onWatchChange}
        playable={playable}
        onPlayableChange={onPlayableChange}
        canClear={canClear}
        onClear={onClear}
        disabled={disabled}
      />

      <div className="hidden md:block">
        <SortMenu
          sort={sort}
          onSortChange={onSortChange}
          onShuffle={onShuffle}
          disabled={disabled}
        />
      </div>

      <Tooltip content="カードの大きさ">
        <ZoomSlider
          zoom={zoom}
          onZoomChange={onZoomChange}
          className="hidden w-24 xl:flex"
        />
      </Tooltip>

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="secondary"
            aria-label="表示と並び順"
            className="px-2.5 xl:hidden"
          >
            <SlidersHorizontal />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-80">
          <div className="space-y-4">
            <div className="md:hidden">
              <CompactSortControls
                name="folder-compact-sort"
                sort={sort}
                onSortChange={onSortChange}
                onShuffle={onShuffle}
                disabled={disabled}
              />
            </div>

            <fieldset className="xl:hidden">
              <legend className="mb-1 text-xs font-semibold text-fg-muted uppercase">
                カードの大きさ
              </legend>
              <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />
            </fieldset>
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
