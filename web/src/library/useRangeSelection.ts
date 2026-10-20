import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import type { LibraryItem } from "../api/client";
import { itemsBetween, type LibrarySelection, setItems } from "./selection";

/**
 * Shift with a check or a selection-mode click gives every item from the previous one to this
 * one the same state. Shift is read when the press starts, so card callbacks keep their signature.
 */
export function useRangeSelection(
  items: readonly LibraryItem[],
  setSelection: Dispatch<SetStateAction<LibrarySelection>>,
) {
  const shift = useRef(false);
  useEffect(() => {
    const note = (event: PointerEvent | KeyboardEvent) => {
      shift.current = event.shiftKey;
    };
    document.addEventListener("pointerdown", note, true);
    document.addEventListener("keydown", note, true);
    return () => {
      document.removeEventListener("pointerdown", note, true);
      document.removeEventListener("keydown", note, true);
    };
  }, []);

  const latest = useRef(items);
  useLayoutEffect(() => {
    latest.current = items;
  });
  const anchor = useRef<string | null>(null);

  const select = useCallback(
    (
      key: string,
      selected: boolean,
      single: (current: LibrarySelection) => LibrarySelection,
    ) => {
      const from = anchor.current;
      anchor.current = key;
      const range =
        shift.current && from !== null ? itemsBetween(latest.current, from, key) : null;
      setSelection((current) =>
        range === null ? single(current) : setItems(current, range, selected),
      );
    },
    [setSelection],
  );
  const point = useCallback((key: string | null) => {
    anchor.current = key;
  }, []);
  return { select, point };
}
