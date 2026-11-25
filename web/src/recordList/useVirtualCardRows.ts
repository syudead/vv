import {
  defaultRangeExtractor,
  type Range,
  useWindowVirtualizer,
} from "@tanstack/react-virtual";
import { type FocusEvent, useCallback, useLayoutEffect, useRef, useState } from "react";

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
 * and the minimum card width token; a focused row stays drawn so its card keeps focus while scrolling.
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

  const rangeExtractor = useCallback((range: Range) => {
    const indexes = defaultRangeExtractor(range);
    const focused = focusedRowRef.current;
    if (focused === null || indexes.includes(focused) || focused >= range.count)
      return indexes;
    return [...indexes, focused].sort((a, b) => a - b);
  }, []);

  const virtualizer = useWindowVirtualizer({
    count: Math.ceil(count / columns),
    estimateSize: () => rowEstimate,
    overscan: CARD_ROW_OVERSCAN,
    scrollMargin,
    rangeExtractor,
  });

  /** Keep the row holding focus drawn; pass to the grid's onFocus and onBlur. */
  const onFocus = useCallback((event: FocusEvent<HTMLElement>) => {
    const row = (event.target as HTMLElement).closest("[data-index]");
    focusedRowRef.current = row === null ? null : Number(row.getAttribute("data-index"));
  }, []);
  const onBlur = useCallback((event: FocusEvent<HTMLElement>) => {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      focusedRowRef.current = null;
    }
  }, []);

  return { gridRef, columns, virtualizer, onFocus, onBlur };
}
