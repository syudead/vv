import { useEffect } from "react";
import { useLocation } from "react-router";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import {
  TagListChanged,
  TagLoadingMore,
  TagLoadMoreFailed,
} from "../recordList/TagListNotices";
import type { MoreState } from "../recordList/usePagedRows";
import { CARD_ROW_OVERSCAN, useVirtualCardRows } from "../recordList/useVirtualCardRows";
import TagCard from "./TagCard";

/** `gap-3`: between columns, and under each row as its padding. */
const GAP = 12;
/** `record-card` (6rem) and the gap under it. */
const ROW_ESTIMATE = 96 + GAP;

/**
 * The tag cards as `CardGrid size="record"` lays them out, drawn a row at a time near the viewport;
 * asks for the next page when its last rows are drawn.
 */
function TagCardGrid({
  tags,
  busy,
  onOpen,
  onNearEnd,
}: {
  tags: readonly Tag[];
  /** More cards are loading at the end. */
  busy: boolean;
  onOpen: (tag: Tag) => void;
  onNearEnd: () => void;
}) {
  const { gridRef, columns, rowCount, virtualizer, onFocus, onBlur, onKeyDown } =
    useVirtualCardRows({
      count: tags.length,
      minWidthToken: "--spacing-card-record",
      rowEstimate: ROW_ESTIMATE,
      gap: GAP,
    });
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const items = virtualizer.getVirtualItems();
  const lastDrawn = items.at(-1)?.index ?? -1;
  // Every new page is checked too: one read under other criteria can bring a cursor with the same rows drawn.
  useEffect(() => {
    if (lastDrawn >= rowCount - CARD_ROW_OVERSCAN) onNearEnd();
  }, [tags, lastDrawn, rowCount, onNearEnd]);

  const top = virtualizer.options.scrollMargin;
  return (
    <div
      ref={gridRef}
      role="region"
      aria-label={t.tags.list.cards}
      aria-busy={busy || undefined}
      data-slot="card-grid"
      className="relative"
      // The last row's gap is not part of the list.
      style={{ height: Math.max(0, virtualizer.getTotalSize() - GAP) }}
      onFocus={onFocus}
      onBlur={onBlur}
      onKeyDown={onKeyDown}
    >
      {items.map((item) => (
        <div
          key={item.key}
          ref={virtualizer.measureElement}
          data-index={item.index}
          className="absolute inset-x-0 top-0 grid gap-3 pb-3"
          style={{
            transform: `translateY(${String(item.start - top)}px)`,
            gridTemplateColumns: `repeat(${String(columns)}, minmax(0, 1fr))`,
          }}
        >
          {tags.slice(item.index * columns, (item.index + 1) * columns).map((tag) => (
            <TagCard key={tag.id} tag={tag} backTo={backTo} onOpen={onOpen} />
          ))}
        </div>
      ))}
    </div>
  );
}

/** The grid and what ends it: more cards loading, a failed page, or a list that changed under paging. */
export function TagCards({
  tags,
  tail,
  onOpen,
  onNearEnd,
  onRetryMore,
  onReload,
}: {
  tags: readonly Tag[];
  tail: MoreState;
  onOpen: (tag: Tag) => void;
  onNearEnd: () => void;
  onRetryMore: () => void;
  onReload: () => void;
}) {
  return (
    <>
      <TagCardGrid
        tags={tags}
        busy={tail.kind === "loading"}
        onOpen={onOpen}
        onNearEnd={onNearEnd}
      />
      {tail.kind === "loading" && <TagLoadingMore />}
      {tail.kind === "failed" && (
        <TagLoadMoreFailed reason={tail.error} onRetry={onRetryMore} />
      )}
      {tail.kind === "inconsistent" && <TagListChanged onReload={onReload} />}
    </>
  );
}
