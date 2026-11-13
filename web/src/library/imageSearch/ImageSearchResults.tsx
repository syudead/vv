import { useLayoutEffect, useState } from "react";

import type { ImageSearchItem, ImageSearchResult } from "../../api/imageSearch";
import { t } from "../../i18n";
import { returnState } from "../../player/pageDecisions";
import { ResultBand } from "../../ui/patterns/query-results-page";
import { Button } from "../../ui/shadcn/button";
import { bandsWithItems, hasBestCandidate } from "./imageSearchState";
import { MatchRow } from "./MatchRow";

export interface ImageSearchResultsProps {
  /** An answer with at least one item. */
  result: ImageSearchResult;
  /** The library URL the player goes back to. */
  backTo: string;
  /** The history entry the search is kept under, so closing the player restores it. */
  entry: string;
  /** The scroll position to restore when the search came back from history. */
  scrollY: number | undefined;
  /** The keys of the rows collapsed when the search came back from history. */
  initialCollapsed?: ReadonlySet<string>;
  /** Called with the keys of the collapsed rows whenever the owner changes them. */
  onCollapsedChange?: (collapsed: ReadonlySet<string>) => void;
  /** Called before a row opens the player, to keep the scroll position. */
  onOpen: () => void;
}

function rowKey(item: ImageSearchItem): string {
  return `${String(item.video.id)}-${String(item.match.videoId)}`;
}

/** The summary line and the answer's rows in their match-rate bands, strongest first. */
export function ImageSearchResults({
  result,
  backTo,
  entry,
  scrollY,
  initialCollapsed,
  onCollapsedChange,
  onOpen,
}: ImageSearchResultsProps) {
  const text = t.library.imageSearch;
  // Rows are expanded when the answer arrives; the set holds the ones the owner collapsed. Back from
  // the player it starts from the kept set, so the rows have their heights before the scroll applies.
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(
    () => initialCollapsed ?? new Set(),
  );
  const changeCollapsed = (next: ReadonlySet<string>) => {
    setCollapsed(next);
    onCollapsedChange?.(next);
  };
  // Back from the player, the rows are drawn from memory in this commit, so the position applies at once.
  useLayoutEffect(() => {
    if (scrollY !== undefined) window.scrollTo({ top: scrollY, behavior: "auto" });
  }, [scrollY]);

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
                tone={band.id === firstBand ? "primary" : "muted"}
                best={best && item === items[0]}
                expanded={!collapsed.has(key)}
                onExpandedChange={(expanded) => setExpanded(key, expanded)}
                back={back}
                onOpen={onOpen}
              />
            );
          })}
        </ResultBand>
      ))}
    </>
  );
}
