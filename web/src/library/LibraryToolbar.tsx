import {
  ArrowDownAZ,
  ArrowUpDown,
  CalendarArrowDown,
  ChevronDown,
  Grid2X2,
  Grid3X3,
  LayoutGrid,
  ListFilter,
} from "lucide-react";

import type { VideoSort } from "../api/client";
import { cn } from "../lib/cn";
import type { WatchState } from "../lib/format";
import type { Density } from "../preferences/viewPreferences";
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

export type WatchFilter = "all" | WatchState;

export const sortOptions: {
  value: VideoSort;
  label: string;
  icon: typeof ArrowDownAZ;
}[] = [
  { value: "addedDesc", label: "追加が新しい順", icon: CalendarArrowDown },
  { value: "titleAsc", label: "題名順", icon: ArrowDownAZ },
];

const watchOptions: { value: WatchFilter; label: string }[] = [
  { value: "all", label: "すべて" },
  { value: "unwatched", label: "未視聴" },
  { value: "inProgress", label: "視聴途中" },
  { value: "watched", label: "視聴済み" },
];

const densityOptions = [
  { value: "dense", label: "小さく表示", icon: <Grid3X3 /> },
  { value: "standard", label: "標準", icon: <Grid2X2 /> },
  { value: "relaxed", label: "大きく表示", icon: <LayoutGrid /> },
] as const;

export default function LibraryToolbar({
  sort,
  onSortChange,
  watch,
  onWatchChange,
  playableOnly,
  onPlayableOnlyChange,
  density,
  onDensityChange,
}: {
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playableOnly: boolean;
  onPlayableOnlyChange: (value: boolean) => void;
  density: Density;
  onDensityChange: (value: Density) => void;
}) {
  const activeSort = sortOptions.find((option) => option.value === sort) ?? {
    label: "並び順",
  };
  const filterCount = (watch === "all" ? 0 : 1) + (playableOnly ? 1 : 0);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <MenuRoot>
        <MenuTrigger asChild>
          <Button variant="secondary" aria-label={`並び順: ${activeSort.label}`}>
            <ArrowUpDown className="text-fg-muted" />
            <span className="hidden sm:inline">{activeSort.label}</span>
            <ChevronDown className="-mr-1 text-fg-subtle" />
          </Button>
        </MenuTrigger>
        <MenuContent>
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

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="secondary"
            aria-label={
              filterCount > 0 ? `絞り込み（${String(filterCount)} 件適用中）` : "絞り込み"
            }
            className={cn(filterCount > 0 && "border-accent/50 text-fg")}
          >
            <ListFilter className="text-fg-muted" />
            <span className="hidden sm:inline">絞り込み</span>
            {filterCount > 0 && (
              <span className="flex size-5 items-center justify-center rounded-full bg-accent text-[11px] font-semibold text-accent-fg tabular-nums">
                {filterCount}
              </span>
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent>
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-2 text-xs font-medium text-fg-subtle">視聴状態</legend>
            <div className="grid grid-cols-2 gap-1.5">
              {watchOptions.map((option) => (
                <label
                  key={option.value}
                  className={cn(
                    "flex h-9 cursor-pointer items-center justify-center rounded-md border text-sm transition-colors",
                    watch === option.value
                      ? "border-accent bg-accent-soft text-fg"
                      : "border-border text-fg-muted hover:bg-surface-hover hover:text-fg",
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

          <label className="mt-4 flex cursor-pointer items-center justify-between gap-3 text-sm">
            <span className="text-fg">再生できるものだけ</span>
            <span
              className={cn(
                "relative h-5 w-9 rounded-full transition-colors",
                playableOnly ? "bg-accent" : "bg-fg/20",
              )}
            >
              <input
                type="checkbox"
                checked={playableOnly}
                onChange={(event) => onPlayableOnlyChange(event.target.checked)}
                className="peer sr-only"
              />
              <span
                className={cn(
                  "absolute top-0.5 left-0.5 size-4 rounded-full bg-fg shadow-sm transition-transform",
                  playableOnly && "translate-x-4",
                )}
              />
            </span>
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
        <SegmentedControl
          label="表示密度"
          value={density}
          onValueChange={onDensityChange}
          options={densityOptions}
        />
      </div>
    </div>
  );
}
