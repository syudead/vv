import {
  type Dispatch,
  type RefObject,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
} from "react";

import type { LibraryItem } from "../api/client";
import { itemsBetween, type LibrarySelection, setItems } from "./selection";

/**
 * Shift with a check or a selection-mode click gives every item from the previous one to this
 * one the same state. Shift is read when the press starts, so card callbacks keep their signature.
 */
export function useRangeSelection(
  /** The items as displayed, including pending moves, so a range never reaches hidden items. */
  displayed: RefObject<readonly LibraryItem[]>,
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
        shift.current && from !== null
          ? itemsBetween(displayed.current, from, key)
          : null;
      setSelection((current) =>
        range === null ? single(current) : setItems(current, range, selected),
      );
    },
    [displayed, setSelection],
  );
  const point = useCallback((key: string | null) => {
    anchor.current = key;
  }, []);
  return { select, point };
}
