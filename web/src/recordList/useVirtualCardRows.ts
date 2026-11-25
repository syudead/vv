import {
  defaultRangeExtractor,
  type Range,
  useWindowVirtualizer,
} from "@tanstack/react-virtual";
import {
  type FocusEvent,
  type KeyboardEvent,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { flushSync } from "react-dom";

import { rowFocusables } from "./useVirtualTagRows";

/** Rows drawn past each edge of the viewport; also how near the end the next page is asked for. */
export const CARD_ROW_OVERSCAN = 3;

/** A CSS length token in pixels; rem resolves against the root font size. */
function tokenPixels(name: string): number {
  const root = document.documentElement;
  const value = getComputedStyle(root).getPropertyValue(name).trim();
  const number = Number.parseFloat(value);
  if (Number.isNaN(number)) return 0;
  if (value.endsWith("rem"))
    return number * (Number.parseFloat(getComputedStyle(root).fontSize) || 16);
  return number;
}

/** Columns of at least minWidth that fit width with gap between them, as `repeat(auto-fill, minmax(…))` lays out. */
export function cardColumns(width: number, minWidth: number, gap: number): number {
  if (width <= 0 || minWidth <= 0) return 1;
  return Math.max(1, Math.floor((width + gap) / (minWidth + gap)));
}

/**
 * Window-virtualize a grid of fixed-height cards by rows. The column count follows the grid's width
 * and the minimum card width token; a focused row stays drawn so its card keeps focus while scrolling,
 * and Tab past a row's edge draws the next row first so focus moves in list order.
 */
export function useVirtualCardRows({
  count,
  minWidthToken,
  rowEstimate,
  gap,
}: {
  count: number;
  /** The CSS custom property of the minimum card width, such as `--spacing-card-record`. */
  minWidthToken: string;
  /** A row's height with its gap, before it is measured. */
  rowEstimate: number;
  /** The gap between columns in pixels. */
  gap: number;
}) {
  const gridRef = useRef<HTMLDivElement | null>(null);
  const [columns, setColumns] = useState(1);
  const [scrollMargin, setScrollMargin] = useState(0);
  const focusedRowRef = useRef<number | null>(null);
  /** A row drawn for Tab to move into before focus reaches it. */
  const [pinnedRow, setPinnedRow] = useState<number | null>(null);
  const rowCount = Math.ceil(count / columns);

  // Measure after every commit: the header above can change height as its counts and filter load.
  useLayoutEffect(() => {
    const grid = gridRef.current;
    if (grid === null) return;
    const measure = () => {
      setColumns(cardColumns(grid.clientWidth, tokenPixels(minWidthToken), gap));
      setScrollMargin(grid.getBoundingClientRect().top + window.scrollY);
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(grid);
    return () => observer.disconnect();
  });

  // Depends on pinnedRow so pinning a row recomputes the drawn rows.
  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      const kept = [focusedRowRef.current, pinnedRow].filter(
        (row): row is number =>
          row !== null && row < range.count && !indexes.includes(row),
      );
      if (kept.length === 0) return indexes;
      return [...new Set([...indexes, ...kept])].sort((a, b) => a - b);
    },
    [pinnedRow],
  );

  const virtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => rowEstimate,
    overscan: CARD_ROW_OVERSCAN,
    scrollMargin,
    rangeExtractor,
  });

  /** Keep the row holding focus drawn; pass to the grid's onFocus and onBlur. */
  const onFocus = useCallback((event: FocusEvent<HTMLElement>) => {
    const row = (event.target as HTMLElement).closest("[data-index]");
    focusedRowRef.current = row === null ? null : Number(row.getAttribute("data-index"));
    setPinnedRow(null);
  }, []);
  const onBlur = useCallback((event: FocusEvent<HTMLElement>) => {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      focusedRowRef.current = null;
    }
  }, []);

  /** Tab from a row's last card (Shift+Tab from its first) into an undrawn neighbour row; pass to onKeyDown. */
  const onKeyDown = useCallback(
    (event: KeyboardEvent<HTMLElement>) => {
      if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
      if (event.defaultPrevented) return;
      const grid = event.currentTarget;
      const target = event.target as HTMLElement;
      const row = target.closest<HTMLElement>("[data-index]");
      if (row === null || !grid.contains(row)) return;
      const stops = rowFocusables(row);
      if (target !== (event.shiftKey ? stops[0] : stops.at(-1))) return;
      const next = Number(row.dataset.index) + (event.shiftKey ? -1 : 1);
      if (next < 0 || next >= rowCount) return;
      const selector = `[data-index="${String(next)}"]`;
      if (grid.querySelector(selector) !== null) return;
      event.preventDefault();
      flushSync(() => setPinnedRow(next));
      const drawn = grid.querySelector(selector);
      if (drawn === null) return;
      const into = rowFocusables(drawn);
      (event.shiftKey ? into.at(-1) : into[0])?.focus();
    },
    [rowCount],
  );

  return { gridRef, columns, rowCount, virtualizer, onFocus, onBlur, onKeyDown };
}
