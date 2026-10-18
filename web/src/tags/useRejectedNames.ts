import { useCallback, useEffect, useRef, useState } from "react";

import {
  forgetRejectedTagName,
  listRejectedTagNamePage,
  type RejectedTagNameList,
} from "../api/tags";
import { errorText, type UiText } from "../i18n";

const rejectedPageLimit = 100;

export interface RejectedNamesState {
  page: RejectedTagNameList | undefined;
  error: UiText | null;
  morePending: boolean;
  moreError: UiText | null;
  epoch: number;
  reload: () => void;
  loadMore: () => void;
  forget: (name: string) => Promise<void>;
}

export function useRejectedNames(): RejectedNamesState {
  const [page, setPage] = useState<RejectedTagNameList | undefined>(undefined);
  const [error, setError] = useState<UiText | null>(null);
  const [morePending, setMorePending] = useState(false);
  const [moreError, setMoreError] = useState<UiText | null>(null);
  const [epoch, setEpoch] = useState(0);
  const generation = useRef(0);
  const inFlight = useRef<number | null>(null);
  const moreInFlight = useRef(false);
  /** Prevent pending pages from restoring names removed after the first page loaded. */
  const forgotten = useRef(new Set<string>());

  /** Restart at page one and reject stale generations; refresh failure hides old data rather than presenting obsolete names as current. */
  const reload = useCallback(() => {
    generation.current += 1;
    const current = generation.current;
    inFlight.current = current;
    moreInFlight.current = false;
    setError(null);
    setMorePending(false);
    setMoreError(null);
    listRejectedTagNamePage(undefined, rejectedPageLimit)
      .then((first) => {
        if (current !== generation.current) return;
        inFlight.current = null;
        forgotten.current = new Set();
        setPage(first);
        setEpoch((value) => value + 1);
      })
      .catch((failure: unknown) => {
        if (current !== generation.current) return;
        inFlight.current = null;
        setPage(undefined);
        setError(errorText(failure));
      });
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  /** Serialize pages and preserve loaded names on failure; retain first-page totals adjusted by removals instead of stale later-page totals. */
  function loadMore() {
    const cursor = page?.nextCursor;
    if (cursor === undefined || moreInFlight.current) return;
    if (inFlight.current !== null) return;
    const current = generation.current;
    moreInFlight.current = true;
    setMorePending(true);
    setMoreError(null);
    listRejectedTagNamePage(cursor, rejectedPageLimit)
      .then((next) => {
        if (current !== generation.current) return;
        moreInFlight.current = false;
        setMorePending(false);
        setPage((latest) => {
          if (latest === undefined) return latest;
          const known = new Set(latest.items);
          const added = next.items.filter(
            (name) => !known.has(name) && !forgotten.current.has(name),
          );
          return {
            items: [...latest.items, ...added],
            total: latest.total,
            ...(next.nextCursor === undefined ? {} : { nextCursor: next.nextCursor }),
          };
        });
      })
      .catch((failure: unknown) => {
        if (current !== generation.current) return;
        moreInFlight.current = false;
        setMorePending(false);
        setMoreError(errorText(failure));
      });
  }

  /** If removal overlaps first-page refresh, discard and refetch because its count may predate the removal, including unloaded names. */
  async function forget(name: string) {
    const started = generation.current;
    const refreshing = inFlight.current !== null;
    await forgetRejectedTagName(name);
    const overlapped =
      refreshing || started !== generation.current || inFlight.current !== null;
    forgotten.current.add(name);
    setPage((current) => {
      if (current === undefined || !current.items.includes(name)) return current;
      return {
        ...current,
        items: current.items.filter((item) => item !== name),
        total: Math.max(current.total - 1, 0),
      };
    });
    if (overlapped) reload();
  }

  return { page, error, morePending, moreError, epoch, reload, loadMore, forget };
}
