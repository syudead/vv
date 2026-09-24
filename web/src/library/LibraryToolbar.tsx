import * as Slider from "@radix-ui/react-slider";
import { LayoutGrid, List, ListFilter, SlidersHorizontal } from "lucide-react";
import { type RefObject, useState } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { cn } from "../lib/cn";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import Button from "../ui/Button";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import SegmentedControl from "../ui/SegmentedControl";
import Tooltip from "../ui/Tooltip";
import type { HistoryMode } from "./listCriteria";
import SearchBox from "./SearchBox";
import { CompactSortControls, SortMenu } from "./SortControls";

export const watchOptions: { value: WatchFilter; label: string }[] = [
  { value: "all", label: "すべて" },
  { value: "unwatched", label: "未視聴" },
  { value: "inProgress", label: "視聴途中" },
  { value: "watched", label: "視聴済み" },
];

const viewOptions = [
  { value: "grid", label: "グリッド", icon: <LayoutGrid /> },
  { value: "list", label: "リスト", icon: <List /> },
] as const;

export function ZoomSlider({
  zoom,
  onZoomChange,
  className,
}: {
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
  className?: string;
}) {
  return (
    <Slider.Root
      value={[zoom]}
      min={0}
      max={3}
      step={1}
      onValueChange={([next]) => {
        if (next !== undefined) onZoomChange(next as Zoom);
      }}
      className={cn("relative flex h-9 touch-none items-center select-none", className)}
    >
      <Slider.Track className="relative h-1 grow rounded-full bg-border-strong">
        <Slider.Range className="absolute h-full rounded-full bg-accent" />
      </Slider.Track>
      <Slider.Thumb
        aria-label="カードの大きさ"
        className="block size-4 rounded-full bg-fg shadow-card transition-transform hover:scale-110"
      />
    </Slider.Root>
  );
}

export interface FilterMenuProps {
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  /** 検索語・視聴状態・再生可否のどれかが効いているか（「条件を解除」を出す）。 */
  canClear: boolean;
  /** 検索語・視聴状態・再生可否を外す。並べ替えは残す。 */
  onClear: () => void;
  disabled?: boolean;
}

/**
 * FilterMenu は絞り込みのボタンとポップオーバーである（ui-design.md「Filter menu」）。
 * ボタンの数字は視聴状態と再生可否の数だけを数える。検索語は検索欄に見えている。
 */
export function FilterMenu({
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  canClear,
  onClear,
  disabled,
}: FilterMenuProps) {
  const [open, setOpen] = useState(false);
  const filterCount = disabled ? 0 : (watch === "all" ? 0 : 1) + (playable ? 1 : 0);

  return (
    <PopoverRoot open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="secondary"
          disabled={disabled}
          aria-label={
            filterCount > 0 ? `絞り込み（${String(filterCount)} 件適用中）` : "絞り込み"
          }
          className={cn("px-2.5", filterCount > 0 && "bg-accent-soft text-link")}
        >
          <ListFilter />
          {filterCount > 0 && <span className="tabular-nums">{filterCount}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start">
        <fieldset>
          <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
            視聴状態
          </legend>
          <div className="grid grid-cols-2 gap-1">
            {watchOptions.map((option) => (
              <label
                key={option.value}
                className={cn(
                  "flex h-8 cursor-pointer items-center justify-center rounded-md text-sm transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-link",
                  watch === option.value
                    ? "bg-accent text-accent-fg"
                    : "text-fg hover:bg-hover-wash",
                )}
              >
                <input
                  type="radio"
                  name="watch"
                  value={option.value}
                  checked={watch === option.value}
                  onChange={() => onWatchChange(option.value)}
                  className="sr-only"
                />
                {option.label}
              </label>
            ))}
          </div>
        </fieldset>

        <label className="mt-4 flex cursor-pointer items-center gap-2.5 text-sm text-fg">
          <input
            type="checkbox"
            checked={playable}
            onChange={(event) => onPlayableChange(event.target.checked)}
            className="size-4 accent-accent"
          />
          再生できるものだけ
        </label>

        {canClear && (
          <Button
            variant="ghost"
            size="sm"
            className="mt-4 w-full"
            onClick={() => {
              onClear();
              // 閉じるとフォーカスは絞り込みのボタンへ戻る（Radix の既定）。
              setOpen(false);
            }}
          >
            条件を解除
          </Button>
        )}
      </PopoverContent>
    </PopoverRoot>
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
