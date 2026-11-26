import {
  type MutableRefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import { subscribeServerEvents } from "../api/serverEvents";
import type { Tag } from "../api/tags";
import { readTagPages, type TagPagesRead } from "./useTagPage";
import type { TagPageRows } from "./tagPageRows";

/** Read the first count rows again with the shown criteria. */
function reread(shown: TagPageRows, search: string, signal: AbortSignal) {
  return readTagPages(
    shown.query,
    search,
    { cursor: undefined, wanted: shown.rows.length },
    signal,
  );
}

function sameTag(a: Tag, b: Tag): boolean {
  return (
    a.id === b.id &&
    a.name === b.name &&
    a.videoCount === b.videoCount &&
    a.tentative === b.tentative &&
    a.kind?.id === b.kind?.id &&
    a.kind?.name === b.kind?.name &&
    a.synonyms.join("\0") === b.synonyms.join("\0")
  );
}

/** Whether a card or a total the list shows differs from what the server now answers. */
export function differsFromShown(shown: TagPageRows, fresh: TagPagesRead): boolean {
  if (shown.total !== fresh.total || shown.totalAll !== fresh.totalAll) return true;
  if (shown.rows.length !== fresh.rows.length) return true;
  return shown.rows.some((tag, index) => !sameTag(tag, fresh.rows[index]!));
}

/**
 * Show a notice when a `tags` event changed the loaded cards, without moving them. An event during
 * the list's own write, or while a first page loads, is checked once that settles.
 */
export function useTagListChanges({
  pageRef,
  searchRef,
  firstPending,
}: {
  pageRef: MutableRefObject<TagPageRows | undefined>;
  /** The raw search the shown pages were read with. */
  searchRef: MutableRefObject<string>;
  firstPending: boolean;
}) {
  const [changed, setChanged] = useState(false);
  const [writes, setWrites] = useState(0);
  const writesRef = useRef(0);
  const firstPendingRef = useRef(firstPending);
  const waitingRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  useLayoutEffect(() => {
    firstPendingRef.current = firstPending;
  });

  /** Compare shown (by default the rows on screen) with a fresh read of the same rows. */
  const check = useCallback(
    (shown = pageRef.current, search = searchRef.current) => {
      if (shown === undefined) return;
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      reread(shown, search, controller.signal).then(
        (fresh) => {
          // A reload under other criteria answers for itself.
          if (controller.signal.aborted || pageRef.current?.query !== shown.query) return;
          if (differsFromShown(shown, fresh)) setChanged(true);
        },
        () => undefined,
      );
    },
    [pageRef, searchRef],
  );

  const busy = () => writesRef.current > 0 || firstPendingRef.current;

  const checkWhenIdle = useCallback(() => {
    if (busy()) {
      waitingRef.current = true;
      return;
    }
    check();
  }, [check]);

  useEffect(() => subscribeServerEvents({ tags: checkWhenIdle }), [checkWhenIdle]);

  useEffect(() => {
    if (writes > 0 || firstPending || !waitingRef.current) return;
    waitingRef.current = false;
    check();
  }, [writes, firstPending, check]);

  useEffect(() => () => abortRef.current?.abort(), []);

  /** Run one of the list's own writes; its result is applied to the shown cards before any check. */
  const track = useCallback(<T>(write: Promise<T>): Promise<T> => {
    writesRef.current += 1;
    setWrites(writesRef.current);
    const settle = () => {
      writesRef.current -= 1;
      setWrites(writesRef.current);
    };
    write.then(settle, settle);
    return write;
  }, []);

  /** Forget the notice, as after a reload that shows the current rows. */
  const reset = useCallback(() => {
    abortRef.current?.abort();
    setChanged(false);
  }, []);

  return { changed, check, track, reset };
}
