import { useCallback, useLayoutEffect, useRef, useState } from "react";

import type { Tag } from "../api/tags";
import { useShiftAtPress } from "../library/useRangeSelection";
import { idsBetween, keepLoaded, withoutIds } from "./tagSelection";

const none: ReadonlySet<number> = new Set();

/**
 * The selected tag cards, with the library's checks and Shift ranges. Only loaded cards stay
 * selected, so a reload keeps the selected tags that are still on it.
 */
export function useTagSelection(rows: readonly Tag[]) {
  const [selected, setSelected] = useState<ReadonlySet<number>>(none);
  const shift = useShiftAtPress();
  const anchor = useRef<number | null>(null);
  const rowsRef = useRef(rows);
  const now = useRef(selected);
  useLayoutEffect(() => {
    rowsRef.current = rows;
    now.current = selected;
  });

  useLayoutEffect(() => {
    setSelected((current) => keepLoaded(current, rows));
  }, [rows]);

  /** Shift gives every card from the previously changed one to this one the same state. */
  const change = useCallback(
    (id: number, next: boolean) => {
      const from = anchor.current;
      anchor.current = id;
      const range =
        shift.current && from !== null ? idsBetween(rowsRef.current, from, id) : [id];
      setSelected((current) => {
        const ids = new Set(range.length === 0 ? [id] : range);
        return next ? new Set([...current, ...ids]) : withoutIds(current, ids);
      });
    },
    [shift],
  );

  const toggle = useCallback((id: number) => change(id, !now.current.has(id)), [change]);

  const replace = useCallback((ids: Iterable<number>) => setSelected(new Set(ids)), []);

  const drop = useCallback(
    (ids: ReadonlySet<number>) => setSelected((current) => withoutIds(current, ids)),
    [],
  );

  const clear = useCallback(() => {
    anchor.current = null;
    setSelected(none);
  }, []);

  return { selected, change, toggle, replace, drop, clear };
}

export type TagSelection = ReturnType<typeof useTagSelection>;
