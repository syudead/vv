import { useCallback, useEffect, useRef, useState } from "react";

import {
  type ActivityConditions,
  type ActivityCounts,
  type ActivityEntry,
  type ActivityFeatureCounts,
  type ActivityPage,
  listActivity,
} from "../api/activity";
import { isAborted } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import {
  type ActivityCriteria,
  activityConditions,
  activityCriteriaKey,
} from "./activityCriteria";
import { useWaiters } from "./useWaiters";

export type MoreState = { kind: "idle" } | { kind: "loading" } | { kind: "failed" };

export type ActivityLogState =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: ActivityEntry[];
      /** No cursor means there are no older records. */
      nextCursor?: string;
      /** Status counts apply every filter except status. */
      counts: ActivityCounts;
      /** Feature counts apply every filter except feature. */
      features: ActivityFeatureCounts;
      more: MoreState;
    };

export interface ActivityLog {
  state: ActivityLogState;
  /** No-op while loading or when no next page exists. */
  loadMore: () => void;
  retry: () => void;
  /**
   * Keep current rows while refreshing counts and loaded pages; resolve after refreshes settle,
   * even on failure.
   */
  refresh: () => Promise<void>;
}

type Ready = Extract<ActivityLogState, { kind: "ready" }>;

function readyOf(
  page: ActivityPage,
  items: ActivityEntry[],
  nextCursor: string | undefined,
): Ready {
  return {
    kind: "ready",
    items,
    ...(nextCursor === undefined ? {} : { nextCursor }),
    counts: page.counts,
    features: page.features,
    more: { kind: "idle" },
  };
}

/**
 * Coalesce overlapping refreshes into one trailing reload and keep existing rows on failure; an
 * initial failure requires Retry. Freeze the period boundary across pagination and recompute it on
 * refresh.
 */
export function useActivityLog(criteria: ActivityCriteria): ActivityLog {
  const [state, setState] = useState<ActivityLogState>({ kind: "loading" });
  const stateRef = useRef(state);
  const show = useCallback((next: ActivityLogState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const key = activityCriteriaKey(criteria);
  const criteriaRef = useRef(criteria);
  useEffect(() => {
    criteriaRef.current = criteria;
  });

  // Increment on reload to discard stale responses.
  const generation = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // Refresh the same number of loaded pages.
  const pages = useRef(0);
  const refreshing = useRef(false);
  const again = useRef(false);
  // Reuse these conditions for pagination so the period boundary stays fixed.
  const conditions = useRef<ActivityConditions>({});

  const begin = useCallback(() => {
    generation.current += 1;
    controller.current?.abort();
    controller.current = new AbortController();
    return { gen: generation.current, signal: controller.current.signal };
  }, []);

  const readPage = useCallback(
    (where: ActivityConditions, cursor: string | undefined, signal: AbortSignal) =>
      listActivity({
        ...where,
        ...(cursor === undefined ? {} : { cursor }),
        signal,
      }),
    [],
  );

  // Allow readFirst to call refresh, which is defined later.
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  // Callers waiting for all refreshes to settle.
  const { wait, settle } = useWaiters();

  const readFirst = useCallback(() => {
    const { gen, signal } = begin();
    refreshing.current = false;
    again.current = false;
    pages.current = 0;
    const where = activityConditions(criteriaRef.current, new Date());
    conditions.current = where;
    show({ kind: "loading" });
    readPage(where, undefined, signal).then(
      (page) => {
        if (gen !== generation.current) return;
        pages.current = 1;
        show(readyOf(page, page.items, page.nextCursor));
        if (again.current) void refreshRef.current();
        else settle();
      },
      (error: unknown) => {
        if (isAborted(error) || gen !== generation.current) return;
        show({ kind: "failed" });
        settle();
      },
    );
  }, [begin, readPage, settle, show]);

  const refresh = useCallback((): Promise<void> => {
    if (stateRef.current.kind === "failed") return Promise.resolve();
    const done = wait();
    if (stateRef.current.kind === "loading" || refreshing.current) {
      again.current = true;
      return done;
    }
    const { gen, signal } = begin();
    refreshing.current = true;
    again.current = false;
    const current = stateRef.current;
    if (current.more.kind === "loading") show({ ...current, more: { kind: "idle" } });
    const wanted = Math.max(1, pages.current);
    const where = activityConditions(criteriaRef.current, new Date());
    const finish = () => {
      refreshing.current = false;
      if (again.current) void refresh();
      else settle();
    };
    const read = async () => {
      const first = await readPage(where, undefined, signal);
      const items = [...first.items];
      let last = first;
      let count = 1;
      while (count < wanted && last.nextCursor !== undefined) {
        last = await readPage(where, last.nextCursor, signal);
        items.push(...last.items);
        count += 1;
      }
      return { first, last, items, count };
    };
    read().then(
      ({ first, last, items, count }) => {
        if (gen !== generation.current) return;
        pages.current = count;
        conditions.current = where;
        show(readyOf(first, items, last.nextCursor));
        finish();
      },
      (error: unknown) => {
        if (isAborted(error) || gen !== generation.current) return;
        finish();
      },
    );
    return done;
  }, [begin, readPage, settle, show, wait]);

  useEffect(() => {
    refreshRef.current = refresh;
  }, [refresh]);

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (
      current.kind !== "ready" ||
      current.nextCursor === undefined ||
      current.more.kind === "loading" ||
      refreshing.current
    )
      return;
    const gen = generation.current;
    const signal = controller.current?.signal ?? new AbortController().signal;
    show({ ...current, more: { kind: "loading" } });
    readPage(conditions.current, current.nextCursor, signal).then(
      (page) => {
        const shown = stateRef.current;
        if (gen !== generation.current || shown.kind !== "ready") return;
        pages.current += 1;
        const known = new Set(shown.items.map((item) => item.id));
        const items = [
          ...shown.items,
          ...page.items.filter((item) => !known.has(item.id)),
        ];
        show({
          ...readyOf(page, items, page.nextCursor),
          counts: shown.counts,
          features: shown.features,
        });
      },
      (error: unknown) => {
        const shown = stateRef.current;
        if (isAborted(error) || gen !== generation.current || shown.kind !== "ready")
          return;
        show({ ...shown, more: { kind: "failed" } });
      },
    );
  }, [readPage, show]);

  useEffect(() => {
    readFirst();
  }, [key, readFirst]);

  useEffect(() => {
    const notified = () => void refresh();
    const unsubscribe = subscribeServerEvents({ activity: notified, open: notified });
    return () => {
      unsubscribe();
      generation.current += 1;
      controller.current?.abort();
      settle();
    };
  }, [refresh, settle]);

  return { state, loadMore, retry: readFirst, refresh };
}
