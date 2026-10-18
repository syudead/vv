import { Folder } from "lucide-react";
import { useCallback, useLayoutEffect, useRef, useState } from "react";

import type { VideoTag } from "../api/client";
import { isFolderOnly, showsTentative } from "../api/tagOrder";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { badgeVariants } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import TentativeMark from "../ui/TentativeMark";
import { useTagRowMeasure } from "./TagRowMeasure";
import { computeVisibleTagCount } from "./tagRowOverflow";
import { useHoverOpen } from "./useHoverOpen";

/** Match gap-1 at the 16px root size. */
const GAP_PX = 4;

export interface CardTagRowProps {
  tags: readonly VideoTag[];
  /** In selection mode, chips leave tab order and toggle their card instead of filtering. */
  selectionMode: boolean;
  onPress: (tag: VideoTag) => void;
  onToggleSelection: () => void;
}

/** Keep measured widths except the overflow trigger and a sole oversized visible tag; folder-only assignments use outlined chips. */
function chipClassName(pressable: boolean, shrink = false, folderOnly = false): string {
  return cn(
    badgeVariants({ variant: folderOnly ? "outline" : "secondary" }),
    "max-w-full min-w-0 py-0 font-normal text-muted-foreground has-[>svg]:px-2 [&_svg]:size-3",
    folderOnly && "border-dashed border-input",
    shrink ? "shrink" : "shrink-0",
    pressable &&
      (folderOnly
        ? "hover:border-solid hover:bg-transparent hover:text-foreground"
        : "hover:bg-accent hover:text-foreground"),
  );
}

function chipLabel(tag: VideoTag, folderOnly: boolean): string {
  const row = t.library.tagRow;
  if (folderOnly) {
    return showsTentative(tag)
      ? row.filterByFromFolderTentative(tag.name)
      : row.filterByFromFolder(tag.name);
  }
  return showsTentative(tag) ? row.filterByTentative(tag.name) : row.filterBy(tag.name);
}

/** Wrap full names in overflow lists without changing single-line chip height; align markers with the first line. */
const listChipClassName =
  "h-auto min-h-6 items-start justify-start py-1 text-left whitespace-normal [&_svg]:mt-0.5";

function FolderMark() {
  return <Folder className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />;
}

function TagChip({
  tag,
  pressable,
  shrink,
  inList = false,
  onPress,
}: {
  tag: VideoTag;
  pressable: boolean;
  shrink?: boolean;
  inList?: boolean;
  onPress: () => void;
}) {
  const folderOnly = isFolderOnly(tag);
  const content = (
    <>
      {folderOnly && <FolderMark />}
      <span className={cn("min-w-0", inList ? "break-all" : "truncate")}>{tag.name}</span>
      {showsTentative(tag) && <TentativeMark />}
    </>
  );
  if (!pressable) {
    return (
      <span title={tag.name} className={chipClassName(false, shrink, folderOnly)}>
        {content}
        {folderOnly && <span className="sr-only"> {t.library.tagRow.fromFolder}</span>}
        {showsTentative(tag) && (
          <span className="sr-only"> {t.library.tagRow.tentative}</span>
        )}
      </span>
    );
  }
  return (
    <Button
      variant="ghost"
      title={tag.name}
      aria-label={chipLabel(tag, folderOnly)}
      onClick={onPress}
      className={cn(chipClassName(true, shrink, folderOnly), inList && listChipClassName)}
    >
      {content}
    </Button>
  );
}

export default function CardTagRow({
  tags,
  selectionMode,
  onPress,
  onToggleSelection,
}: CardTagRowProps) {
  const rowRef = useRef<HTMLUListElement | null>(null);
  const measureRowRef = useRef<HTMLDivElement | null>(null);
  const overflowMeasureRef = useRef<HTMLSpanElement | null>(null);
  const [visibleCount, setVisibleCount] = useState(tags.length);

  const recompute = useCallback(() => {
    const row = rowRef.current;
    const measureRow = measureRowRef.current;
    const overflowEl = overflowMeasureRef.current;
    if (row === null || measureRow === null || overflowEl === null) return;
    const available = row.clientWidth;
    const widths = Array.from(measureRow.children).map(
      (child) => (child as HTMLElement).getBoundingClientRect().width,
    );
    const overflowWidth = overflowEl.getBoundingClientRect().width;
    setVisibleCount(computeVisibleTagCount(widths, overflowWidth, GAP_PX, available));
  }, []);

  // Measure before paint to avoid a one-frame row size change.
  // Tag changes rerender the measurement children and require another measurement.
  useLayoutEffect(() => {
    recompute();
  }, [recompute, tags]);

  // Share one ResizeObserver per list; outside the provider only the initial layout measurement runs.
  const observeRow = useTagRowMeasure();
  useLayoutEffect(() => {
    const row = rowRef.current;
    if (row === null || observeRow === null) return;
    return observeRow(row, recompute);
  }, [observeRow, recompute]);

  const pressable = !selectionMode;
  const clampedVisible = Math.min(visibleCount, tags.length);
  const hidden = tags.slice(clampedVisible);
  const visible = tags.slice(0, clampedVisible);
  // Derive overflow from current tags, and reset open state when overflow disappears or selection starts.
  const hasList = pressable && hidden.length > 0;
  const { open, setOpen, triggerHandlers, contentHandlers, lastOpenedByHover } =
    useHoverOpen(hasList);
  // Allow the sole visible tag to shrink when even its natural width cannot fit.
  const forcedShrink = visible.length === 1 && hidden.length > 0;

  return (
    <div
      onClick={selectionMode ? onToggleSelection : undefined}
      className={selectionMode ? "cursor-pointer" : undefined}
    >
      <ul
        ref={rowRef}
        aria-label={t.library.tagRow.label}
        className="flex flex-nowrap items-center gap-1 overflow-hidden"
      >
        {visible.map((tag) => (
          <li
            key={tag.id}
            className={cn("min-w-0 max-w-full", !forcedShrink && "shrink-0")}
          >
            <TagChip
              tag={tag}
              pressable={pressable}
              shrink={forcedShrink}
              onPress={() => onPress(tag)}
            />
          </li>
        ))}
        {hidden.length > 0 &&
          (pressable ? (
            <li className="shrink-0">
              <Popover open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>
                  {}
                  <Button
                    variant="ghost"
                    aria-label={t.library.tagRow.showMore(hidden.length)}
                    className={cn(
                      chipClassName(true),
                      "data-open:bg-accent data-open:text-foreground",
                    )}
                    {...triggerHandlers}
                  >
                    <span className="tabular-nums">
                      {t.library.tagRow.more(hidden.length)}
                    </span>
                  </Button>
                </PopoverTrigger>
                {}
                {/* Hover opening preserves focus; explicit opening focuses the first chip and restores the trigger on close. */}
                <PopoverContent
                  align="start"
                  className="w-auto max-w-popover p-2"
                  onOpenAutoFocus={(event) => {
                    if (lastOpenedByHover()) event.preventDefault();
                  }}
                  onCloseAutoFocus={(event) => {
                    if (lastOpenedByHover()) event.preventDefault();
                  }}
                  {...contentHandlers}
                >
                  <ul className="flex min-h-0 flex-wrap gap-1 overflow-y-auto">
                    {hidden.map((tag) => (
                      <li key={tag.id} className="max-w-full min-w-0">
                        <TagChip
                          tag={tag}
                          pressable
                          inList
                          onPress={() => {
                            setOpen(false);
                            onPress(tag);
                          }}
                        />
                      </li>
                    ))}
                  </ul>
                </PopoverContent>
              </Popover>
            </li>
          ) : (
            <li className="shrink-0">
              <span className={chipClassName(false)}>
                <span className="tabular-nums">
                  {t.library.tagRow.more(hidden.length)}
                </span>
              </span>
            </li>
          ))}
      </ul>

      {}
      <div
        aria-hidden="true"
        // Clip the measurement row at zero width to prevent horizontal page overflow while retaining intrinsic measurements.
        className="pointer-events-none invisible absolute flex w-0 flex-nowrap items-center gap-1 overflow-hidden"
      >
        <div ref={measureRowRef} className="flex flex-nowrap items-center gap-1">
          {tags.map((tag) => (
            <span key={tag.id} className={chipClassName(true, false, isFolderOnly(tag))}>
              {isFolderOnly(tag) && <FolderMark />}
              <span>{tag.name}</span>
              {showsTentative(tag) && <TentativeMark />}
            </span>
          ))}
        </div>
        <span ref={overflowMeasureRef} className={chipClassName(true)}>
          <span className="tabular-nums">{t.library.tagRow.more(tags.length)}</span>
        </span>
      </div>
    </div>
  );
}
