import * as Slider from "@radix-ui/react-slider";
import {
  ArrowDownAZ,
  CalendarArrowDown,
  ChevronDown,
  LayoutGrid,
  List,
  ListFilter,
  SlidersHorizontal,
} from "lucide-react";

import type { VideoSort } from "../api/client";
import { cn } from "../lib/cn";
import type { WatchState } from "../lib/format";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import Button from "../ui/Button";
import {
  MenuContent,
  MenuLabel,
  MenuRadioGroup,
  MenuRadioItem,
  MenuRoot,
  MenuTrigger,
} from "../ui/Menu";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import SegmentedControl from "../ui/SegmentedControl";
import Tooltip from "../ui/Tooltip";
import SearchBox from "./SearchBox";

export type WatchFilter = "all" | WatchState;

export const sortOptions: {
  value: VideoSort;
  label: string;
  icon: typeof ArrowDownAZ;
}[] = [
  { value: "addedDesc", label: "追加日", icon: CalendarArrowDown },
  { value: "titleAsc", label: "題名", icon: ArrowDownAZ },
];

const watchOptions: { value: WatchFilter; label: string }[] = [
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

export interface LibraryToolbarProps {
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playableOnly: boolean;
  onPlayableOnlyChange: (value: boolean) => void;
  view: ViewMode;
  onViewChange: (value: ViewMode) => void;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/** LibraryToolbar はライブラリ操作をトップバー内の 1 行に集める。 */
export default function LibraryToolbar({
  sort,
  onSortChange,
  watch,
  onWatchChange,
  playableOnly,
  onPlayableOnlyChange,
  view,
  onViewChange,
  zoom,
  onZoomChange,
}: LibraryToolbarProps) {
  const activeSort = sortOptions.find((option) => option.value === sort) ?? {
    label: "並び順",
  };
  const filterCount = (watch === "all" ? 0 : 1) + (playableOnly ? 1 : 0);

  return (
    <div className="flex min-w-0 flex-1 items-center justify-center gap-1.5">
      <SearchBox className="min-w-20 flex-1 sm:min-w-40 sm:max-w-md" />

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="secondary"
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
                    "flex h-8 cursor-pointer items-center justify-center rounded-md text-sm transition-colors",
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
              checked={playableOnly}
              onChange={(event) => onPlayableOnlyChange(event.target.checked)}
              className="size-4 accent-accent"
            />
            再生できるものだけ
          </label>

          {filterCount > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-4 w-full"
              onClick={() => {
                onWatchChange("all");
                onPlayableOnlyChange(false);
              }}
            >
              絞り込みを解除
            </Button>
          )}
        </PopoverContent>
      </PopoverRoot>

      <div className="hidden md:block">
        <MenuRoot>
          <MenuTrigger asChild>
            <Button variant="secondary" aria-label={`並び順: ${activeSort.label}`}>
              {activeSort.label}
              <ChevronDown className="-mr-1 text-fg-muted" />
            </Button>
          </MenuTrigger>
          <MenuContent align="start">
            <MenuLabel>並び順</MenuLabel>
            <MenuRadioGroup value={sort} onValueChange={onSortChange}>
              {sortOptions.map((option) => (
                <MenuRadioItem key={option.value} value={option.value}>
                  <option.icon />
                  {option.label}
                </MenuRadioItem>
              ))}
            </MenuRadioGroup>
          </MenuContent>
        </MenuRoot>
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
        <PopoverContent align="end" className="w-64">
          <div className="space-y-4">
            <fieldset className="md:hidden">
              <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
                並び順
              </legend>
              <div className="grid grid-cols-2 gap-1">
                {sortOptions.map((option) => (
                  <label
                    key={option.value}
                    className={cn(
                      "flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md text-sm transition-colors",
                      sort === option.value
                        ? "bg-accent text-accent-fg"
                        : "text-fg hover:bg-hover-wash",
                    )}
                  >
                    <input
                      type="radio"
                      name="compact-sort"
                      value={option.value}
                      checked={sort === option.value}
                      onChange={() => onSortChange(option.value)}
                      className="sr-only"
                    />
                    <option.icon className="size-4" />
                    {option.label}
                  </label>
                ))}
              </div>
            </fieldset>

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
