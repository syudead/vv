import {
  defaultRangeExtractor,
  useWindowVirtualizer,
  type Range,
} from "@tanstack/react-virtual";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
} from "react";
import { flushSync } from "react-dom";

import type { TagRowRefs } from "./TagRow";

export type FocusTarget = "rename" | "synonyms" | "name" | "menu";

/** Estimate ordinary rows, then measure expanded synonym/error rows after rendering. */
const ROW_ESTIMATE = 49;

export const ROW_OVERSCAN = 8;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** Exclude CSS-hidden compact/desktop actions; environments without checkVisibility, including jsdom, assume visible. */
export function isShown(element: HTMLElement): boolean {
  return "checkVisibility" in element ? element.checkVisibility() : true;
}

function rowFocusables(row: Element): HTMLElement[] {
  return [...row.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (element) => element.tabIndex >= 0 && isShown(element),
  );
}

/** Virtualize admin rows while coordinating offscreen focus and Tab boundaries; rowIdAttribute identifies rows and synonymsButton maps the second action. */
export function useVirtualTagRows<R extends { id: number }>({
  visibleRows,
  renamingId,
  scrollMargin,
  rowsTop,
  fallbackFocus,
  rowIdAttribute = "data-tag-id",
}: {
  visibleRows: readonly R[];
  renamingId: number | null;
  scrollMargin: number;
  rowsTop: number;
  fallbackFocus: () => void;
  rowIdAttribute?: string;
}) {
  const fallbackRef = useRef(fallbackFocus);
  useLayoutEffect(() => {
    fallbackRef.current = fallbackFocus;
  });
  function focusFallback() {
    fallbackRef.current();
  }

  const rowRefs = useRef(new Map<number, TagRowRefs>());
  const listRef = useRef<HTMLTableSectionElement | null>(null);
  /** Pin focused or soon-to-be-focused rows, including portaled menus; release when focus leaves the list. */
  const [pinnedRowId, setPinnedRowId] = useState<number | null>(null);
  /** Track focus in the React tree, including menus opened from rows. */
  const focusInListRef = useRef(false);
  const afterCommitQueue = useRef<(() => void)[]>([]);
  const [afterCommitTick, setAfterCommitTick] = useState(0);
  useEffect(() => {
    if (afterCommitQueue.current.length === 0) return;
    const queued = afterCommitQueue.current;
    afterCommitQueue.current = [];
    setTimeout(() => {
      for (const fn of queued) fn();
    }, 0);
  }, [afterCommitTick]);

  const registerRefs = useCallback((id: number, refs: Partial<TagRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {
      nameLink: null,
      renameButton: null,
      synonymsButton: null,
      menuButton: null,
      actionsButton: null,
    };
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  function releasePinIfFocusOutside() {
    if (focusInListRef.current) return;
    if (listRef.current?.contains(document.activeElement)) return;
    setPinnedRowId(null);
  }

  /** Wait for React commit before a zero-delay focus callback; a timer alone may run against stale DOM. */
  function afterCommit(fn: () => void) {
    afterCommitQueue.current.push(fn);
    setAfterCommitTick((tick) => tick + 1);
  }

  /** Synchronously pin an unrendered loaded row; return its index, or -1 if rendered or absent. */
  function renderRow(id: number): number {
    const index = visibleRowsRef.current.findIndex((tag) => tag.id === id);
    if (index === -1) return -1;
    const drawn = listRef.current?.querySelector(`[${rowIdAttribute}="${String(id)}"]`);
    if (drawn !== null && drawn !== undefined) return -1;
    flushSync(() => setPinnedRowId(id));
    return index;
  }

  /** Render offscreen rows before focusing; already-rendered targets rely on browser focus scrolling. */
  function revealRow(id: number) {
    const index = renderRow(id);
    if (index !== -1) virtualizer.scrollToIndex(index, { align: "auto" });
  }

  function focusRow(id: number, part: FocusTarget) {
    afterCommit(() => {
      revealRow(id);
      const refs = rowRefs.current.get(id);
      const preferred =
        part === "rename"
          ? refs?.renameButton
          : part === "synonyms"
            ? refs?.synonymsButton
            : part === "menu"
              ? refs?.menuButton
              : refs?.nameLink;
      const target = rowActionTarget(refs, preferred);
      if (target === null || target === undefined || !target.isConnected) {
        focusFallback();
      } else target.focus();
    });
  }

  /** Find enabled surviving actions in the original order, next then previous; render offscreen candidates before checking them. */
  function focusAfterRemoval(
    order: readonly R[],
    removedId: number,
    alsoGone: ReadonlySet<number> = new Set(),
    fallback: () => void = focusFallback,
  ) {
    const index = order.findIndex((tag) => tag.id === removedId);
    const stays = (tag: R) => tag.id !== removedId && !alsoGone.has(tag.id);
    const candidates = [
      ...order.slice(index + 1).filter(stays),
      ...order.slice(0, Math.max(index, 0)).reverse().filter(stays),
    ];
    afterCommit(() => {
      const listed = new Set(visibleRowsRef.current.map((tag) => tag.id));
      for (const tag of candidates) {
        if (!listed.has(tag.id)) continue;
        const index = renderRow(tag.id);
        const refs = rowRefs.current.get(tag.id);
        const button = rowActionTarget(refs, refs?.renameButton);
        if (button?.isConnected === true && !(button as HTMLButtonElement).disabled) {
          if (index !== -1) virtualizer.scrollToIndex(index, { align: "auto" });
          button.focus();
          return;
        }
      }
      fallback();
    });
  }

  /** Route hidden desktop actions to the visible compact Actions trigger. */
  function rowActionTarget(
    refs: TagRowRefs | undefined,
    preferred: HTMLElement | null | undefined,
  ): HTMLElement | null | undefined {
    if (preferred === null || preferred === undefined) return preferred;
    if (preferred === refs?.nameLink || !preferred.isConnected) return preferred;
    return isShown(preferred) ? preferred : refs?.actionsButton;
  }

  const visibleRowsRef = useRef<readonly R[]>(visibleRows);
  useLayoutEffect(() => {
    visibleRowsRef.current = visibleRows;
  }, [visibleRows]);

  const pinnedIndex = useMemo(
    () =>
      pinnedRowId === null ? -1 : visibleRows.findIndex((tag) => tag.id === pinnedRowId),
    [visibleRows, pinnedRowId],
  );
  /** Keep renaming rows mounted even when focus leaves the list so sorting/filtering cannot destroy drafts. */
  const renamingIndex = useMemo(
    () =>
      renamingId === null ? -1 : visibleRows.findIndex((tag) => tag.id === renamingId),
    [visibleRows, renamingId],
  );
  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      const extra = [pinnedIndex, renamingIndex].filter(
        (index, i, all) =>
          index >= 0 &&
          index < range.count &&
          !indexes.includes(index) &&
          all.indexOf(index) === i,
      );
      if (extra.length === 0) return indexes;
      return [...indexes, ...extra].sort((a, b) => a - b);
    },
    [pinnedIndex, renamingIndex],
  );
  const getItemKey = useCallback(
    (index: number) => visibleRows[index]!.id,
    [visibleRows],
  );
  const virtualizer = useWindowVirtualizer({
    count: visibleRows.length,
    estimateSize: () => ROW_ESTIMATE,
    overscan: ROW_OVERSCAN,
    scrollMargin,
    scrollPaddingStart: rowsTop,
    rangeExtractor,
    getItemKey,
  });
  const virtualItems = virtualizer.getVirtualItems();

  /** Pin focused rows so scrolling cannot drop focus to body. */
  function handleListFocus(event: FocusEvent<HTMLTableSectionElement>) {
    focusInListRef.current = true;
    const row = (event.target as Element).closest<HTMLElement>(`[${rowIdAttribute}]`);
    if (row === null || !event.currentTarget.contains(row)) return;
    setPinnedRowId(Number(row.getAttribute(rowIdAttribute)));
  }

  /** Release pins only after focus truly leaves the list, preserving portaled menus and window blur; stale pins otherwise distort tab order. */
  function handleListBlur() {
    focusInListRef.current = false;
    setTimeout(releasePinIfFocusOutside, 0);
  }

  /** Render adjacent rows at virtual Tab boundaries; only actual list endpoints allow native Tab to leave. */
  function handleListKeyDown(event: KeyboardEvent<HTMLTableSectionElement>) {
    if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
    if (event.defaultPrevented) return;
    const list = event.currentTarget;
    const target = event.target as HTMLElement;
    const row = target.closest<HTMLElement>("[data-index]");
    if (row === null || !list.contains(row)) return;
    const focusables = rowFocusables(row);
    const edge = event.shiftKey ? focusables[0] : focusables[focusables.length - 1];
    if (target !== edge) return;
    const nextIndex = Number(row.dataset.index) + (event.shiftKey ? -1 : 1);
    const next = visibleRows[nextIndex];
    if (next === undefined) return;
    if (list.querySelector(`[data-index="${String(nextIndex)}"]`) !== null) return;
    event.preventDefault();
    revealRow(next.id);
    const nextRow = list.querySelector(`[data-index="${String(nextIndex)}"]`);
    if (nextRow === null) return;
    const nextFocusables = rowFocusables(nextRow);
    const into = event.shiftKey
      ? nextFocusables[nextFocusables.length - 1]
      : nextFocusables[0];
    into?.focus();
  }

  return {
    listRef,
    registerRefs,
    visibleRowsRef,
    virtualizer,
    virtualItems,
    afterCommit,
    focusRow,
    focusAfterRemoval,
    releasePinIfFocusOutside,
    handleListFocus,
    handleListBlur,
    handleListKeyDown,
  };
}
