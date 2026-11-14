import { useLayoutEffect, useState } from "react";

import type { ImageSearchItem, ImageSearchResult } from "../../api/imageSearch";
import { t } from "../../i18n";
import { returnState } from "../../player/pageDecisions";
import { ResultBand } from "../../ui/patterns/query-results-page";
import { Button } from "../../ui/shadcn/button";
import { bandsWithItems, hasBestCandidate } from "./imageSearchState";
import { MatchRow } from "./MatchRow";

/** What the owner left on the results before opening the player, restored on the way back. */
export interface ResultsView {
  scrollY?: number;
  /** The keys of the collapsed rows. */
  collapsed?: ReadonlySet<string>;
  /** The frame last focused in each row's strip, by row key. */
  strips?: ReadonlyMap<string, number>;
  /** The control that opened the player. */
  opener?: string;
}

export interface ImageSearchResultsProps {
  /** An answer with at least one item. */
  result: ImageSearchResult;
  /** The screen's URL the player goes back to. */
  backTo: string;
  /** The token the search is kept under, so closing the player restores it. */
  entry: string;
  /** The view when the search came back from history. */
  restored?: ResultsView;
  /** Called with the keys of the collapsed rows whenever the owner changes them. */
  onCollapsedChange?: (collapsed: ReadonlySet<string>) => void;
  /** Called when the owner focuses another frame of a row's strip. */
  onStripChange?: (row: string, frame: number) => void;
  /** Called with the control's name before a row opens the player, to keep the view. */
  onOpen: (opener: string) => void;
}

function rowKey(item: ImageSearchItem): string {
  return `${String(item.video.id)}-${String(item.match.videoId)}`;
}

/** Focuses the control that opened the player, or the first control inside the element so named. */
function focusOpener(opener: string) {
  const named = document.querySelector<HTMLElement>(`[data-opener="${opener}"]`);
  const target = named?.matches("a, button")
    ? named
    : named?.querySelector<HTMLElement>("a, button");
  target?.focus({ preventScroll: true });
}

/** The summary line and the answer's rows in their match-rate bands, strongest first. */
export function ImageSearchResults({
  result,
  backTo,
  entry,
  restored,
  onCollapsedChange,
  onStripChange,
  onOpen,
}: ImageSearchResultsProps) {
  const text = t.library.imageSearch;
  // Rows are expanded when the answer arrives; the set holds the ones the owner collapsed. Back from
  // the player it starts from the kept set, so the rows have their heights before the scroll applies.
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(
    () => restored?.collapsed ?? new Set(),
  );
  const changeCollapsed = (next: ReadonlySet<string>) => {
    setCollapsed(next);
    onCollapsedChange?.(next);
  };
  // Back from the player, the rows are drawn from memory in this commit, so the position applies at once.
  const scrollY = restored?.scrollY;
  const opener = restored?.opener;
  useLayoutEffect(() => {
    if (scrollY !== undefined) window.scrollTo({ top: scrollY, behavior: "auto" });
    if (opener !== undefined) focusOpener(opener);
  }, [scrollY, opener]);

  const back = returnState(backTo, entry);
  const { items } = result;
  const best = hasBestCandidate(items);
  const firstBand = result.bands[0]?.id;
  const allCollapsed = items.every((item) => collapsed.has(rowKey(item)));
  const setExpanded = (key: string, expanded: boolean) => {
    const next = new Set(collapsed);
    if (expanded) next.delete(key);
    else next.add(key);
    changeCollapsed(next);
  };

  return (
    <>
      <div className="flex min-h-8 items-center gap-2">
        <p role="status" className="min-w-0 flex-1 text-sm text-muted-foreground">
          {text.videos(items.length)}
        </p>
        {items.length > 1 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() =>
              changeCollapsed(allCollapsed ? new Set() : new Set(items.map(rowKey)))
            }
          >
            {allCollapsed ? text.expandAll : text.collapseAll}
          </Button>
        )}
      </div>
      {bandsWithItems(result).map(({ band, items: rows }) => (
        <ResultBand
          key={band.id}
          heading={text.bands[band.id]}
          detail={text.bandRange(band.minPercent, band.maxPercent)}
          count={text.videos(rows.length)}
        >
          {rows.map((item) => {
            const key = rowKey(item);
            return (
              <MatchRow
                key={key}
                item={item}
                rowKey={key}
                tone={band.id === firstBand ? "primary" : "muted"}
                best={best && item === items[0]}
                expanded={!collapsed.has(key)}
                onExpandedChange={(expanded) => setExpanded(key, expanded)}
                back={back}
                onOpen={onOpen}
                stripFrame={restored?.strips?.get(key)}
                onStripFrameChange={(frame) => onStripChange?.(key, frame)}
              />
            );
          })}
        </ResultBand>
      ))}
    </>
  );
}
